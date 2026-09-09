package api

import (
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"

	"synodl/server/internal/httpx"
	"synodl/server/internal/k8s"
	"synodl/server/internal/library"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
	"synodl/server/internal/ytdl"
)

// Direct upload into the library (spec 1022).
//
// This is the one route that puts file CONTENT on the NAS, so its whole job is
// to be narrow. The destination is never taken from the request: the server
// composes it from a parent it already knows, a title it sanitises, and a season
// it formats itself. Two client-supplied strings reach the path — the title and
// the file name — and both are validated as single segments before use.

// uploadField is how many bytes of a non-file part we will read. The fields are
// a title, a parent and a season; anything longer is malformed, not a title.
const uploadField = 4 << 10

// defaultUploadMaxMB backs up config's own clamp. Config.Load never yields a
// non-positive cap, but Deps.Cfg can be built in code, and a zero there would
// make MaxBytesReader reject every upload with "request body too large" — a
// baffling failure for a correct request.
const defaultUploadMaxMB = 10240

func (d Deps) uploadCapBytes() int64 {
	mb := d.Cfg.UploadMaxMB
	if mb < 1 {
		mb = defaultUploadMaxMB
	}
	return int64(mb) << 20
}

// handleUpload streams one file into the library.
//
// The request is read with MultipartReader rather than ParseMultipartForm.
// That matters: ParseMultipartForm spills parts over its memory limit into
// TEMPORARY FILES ON DISK, which would both break the streaming requirement and
// put user content on a server that persists nothing. MultipartReader hands us
// the part as a stream, which goes straight out to the NAS.
//
// The client must therefore send the fields BEFORE the file part, which is also
// what lets an invalid title be refused without reading the body at all.
func handleUpload(d Deps) http.Handler {
	return d.requireUser(func(w http.ResponseWriter, r *http.Request, u *store.User) {
		// Cap first, so an oversized body is cut off rather than streamed whole.
		r.Body = http.MaxBytesReader(w, r.Body, d.uploadCapBytes())

		mr, err := r.MultipartReader()
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "expected a multipart upload")
			return
		}

		var kind, title, season, size, overwrite string
		// Music (spec 1040). Three more client strings that become path
		// segments; every one of them goes through the same sanitise-or-refuse
		// rule the file name already gets.
		var track, artist, album string
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				httpx.Error(w, http.StatusBadRequest, "no file in the upload")
				return
			}
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "malformed upload")
				return
			}
			if part.FormName() == "file" {
				d.streamUploadedFile(w, r, u, uploadRequest{
					Kind: kind, Title: title, Season: season, Size: size,
					Overwrite: overwrite, Track: track, Artist: artist, Album: album,
				}, part)
				return
			}
			val, err := io.ReadAll(io.LimitReader(part, uploadField))
			_ = part.Close()
			if err != nil {
				httpx.Error(w, http.StatusBadRequest, "malformed upload")
				return
			}
			switch part.FormName() {
			case "kind":
				kind = string(val)
			case "title":
				title = string(val)
			case "season":
				season = string(val)
			case "overwrite":
				// Only ever set deliberately by the client, to replace a partial
				// file left behind by an interrupted upload.
				overwrite = string(val)
			case "size":
				// Sent ahead of the file so the NAS request can declare an exact
				// Content-Length (DSM rejects a chunked upload body).
				size = string(val)
			case "track":
				track = string(val)
			case "artist":
				artist = string(val)
			case "album":
				album = string(val)
			}
		}
	})
}

// uploadRequest is everything the fields before the file said.
//
// A struct rather than eight positional strings: the music fields (spec 1040)
// took the count past the point where a caller could be trusted to keep the
// order, and a silently swapped title and artist is a file in the wrong folder
// rather than a compile error.
type uploadRequest struct {
	Kind      string
	Title     string
	Season    string
	Size      string
	Overwrite string
	// Music only.
	Track  string
	Artist string
	Album  string
}

// streamUploadedFile validates everything, makes the folders, and pipes the part
// to the NAS. Nothing here is buffered.
func (d Deps) streamUploadedFile(
	w http.ResponseWriter, r *http.Request, u *store.User,
	req uploadRequest, part *multipart.Part,
) {
	kind, title, season, size, overwrite := req.Kind, req.Title, req.Season, req.Size, req.Overwrite
	name := strings.TrimSpace(part.FileName())
	// The file name is client-supplied text that ends up in a path on the NAS,
	// and it is guarded in two layers. Go's multipart reader has already based
	// the name, so "../../etc/passwd.mkv" arrives as "passwd.mkv" — but it does
	// NOT touch a backslash or a bare "..", both of which survive verbatim. This
	// check catches what Go leaves, and REFUSES rather than repairs: a silently
	// rewritten name would be written to wherever the rewrite landed.
	if !library.ValidUploadName(name) {
		httpx.Error(w, http.StatusBadRequest, "that file name cannot be used")
		return
	}
	// Restricting the types is what keeps this a media upload rather than a
	// general write-anything-to-the-NAS capability. Checked PER KIND (spec 1040):
	// a film upload has no business accepting an .mp3, and a music upload none
	// accepting an .mkv, so each library keeps holding what it is for.
	if !library.AllowedUploadTypeFor(library.UploadKind(kind), name) {
		httpx.Error(w, http.StatusUnsupportedMediaType,
			"that kind of file cannot be uploaded here")
		return
	}

	parent, ok := d.uploadParent(kind)
	if !ok {
		httpx.JSON(w, http.StatusConflict, map[string]any{"error": "parent_unset"})
		return
	}

	// Two shapes, one rule: the SERVER composes the destination from a parent it
	// already holds plus values it has sanitised itself. A client supplies no
	// path in either case.
	var folder, seasonFolder string
	if library.IsMusicKind(library.UploadKind(kind)) {
		// Artist / Album / Track.ext — the same layout spec 0012's download
		// recipe produces, so an uploaded track and a downloaded one sit together
		// rather than in two parallel shapes (FR-005).
		artistFolder, albumFolder, valid := library.MusicFolders(req.Artist, req.Album)
		if !valid {
			httpx.Error(w, http.StatusBadRequest, "an artist is required")
			return
		}
		stored, valid := library.MusicFileName(req.Track, name)
		if !valid {
			httpx.Error(w, http.StatusBadRequest, "a track name is required")
			return
		}
		// The file takes the TRACK's name, not the one the phone happened to give
		// it: a media server pairs a .lrc to its audio by identical base name, so
		// uploading them under two different names is the same as not uploading
		// the lyrics at all (FR-008).
		name = stored
		folder, seasonFolder = artistFolder, albumFolder
	} else {
		// The same naming a download produces, so an uploaded title and a
		// downloaded one are indistinguishable afterwards.
		folder = sanitizeFolderName(library.PlexName(title))
		if !validFolderName(folder) {
			httpx.Error(w, http.StatusBadRequest, "a title is required")
			return
		}
		if n, err := strconv.Atoi(strings.TrimSpace(season)); err == nil && n >= 0 {
			seasonFolder = sanitizeFolderName(library.SeasonFolder(n))
		}
	}
	dest := parent + "/" + folder
	if seasonFolder != "" {
		dest += "/" + seasonFolder
	}
	// The finished path passes the same grant check a download does.
	if !d.destinationAllowed(u, dest) {
		httpx.JSON(w, http.StatusForbidden, map[string]any{"error": "destination_forbidden"})
		return
	}

	// Deliberately checked here, AFTER the name, type, parent and permission
	// checks: those are what the user can act on, and letting a transport detail
	// preempt them would report "missing file size" for a request whose real
	// problem is that no library folder is configured.
	//
	// The byte count must be known before the NAS request is built, because DSM
	// refuses a chunked upload body so the length has to be declared up front.
	// The browser knows it (File.size) and sends it in the field ahead of the
	// file; there is no way to recover it from the part itself.
	nbytes, sizeErr := strconv.ParseInt(strings.TrimSpace(size), 10, 64)
	if sizeErr != nil || nbytes <= 0 {
		httpx.Error(w, http.StatusBadRequest, "the upload is missing its file size")
		return
	}
	if nbytes > d.uploadCapBytes() {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "that file is over the size limit")
		return
	}

	absParent := "/" + parent
	err := d.NAS.Do(r.Context(), func(c syno.Client, sid string) error {
		if e := ensureSubfolder(r.Context(), c, sid, absParent, folder); e != nil {
			return e
		}
		if seasonFolder != "" {
			if e := ensureSubfolder(r.Context(), c, sid, absParent+"/"+folder, seasonFolder); e != nil {
				return e
			}
		}
		return c.UploadFile(r.Context(), sid, "/"+dest, name, nbytes,
			strings.EqualFold(strings.TrimSpace(overwrite), "true"), part)
	})
	if err != nil {
		// A collision is the one failure worth naming precisely: the file is
		// there already, and nothing was overwritten.
		if se := syno.AsError(err); se != nil && se.Code == 414 {
			httpx.JSON(w, http.StatusConflict, map[string]any{"error": "file_exists"})
			return
		}
		writeNASError(w, err)
		return
	}
	// The details go into the file itself, afterwards, in a short-lived worker
	// (spec 1040, FR-012). It cannot happen on the way past: the upload is
	// streamed and never held here, and rewriting tags needs the whole file.
	//
	// BEST EFFORT, and the call says so by ignoring what it returns. A track
	// filed correctly is already usable — folders and names are what a media
	// server matches on — so an upload that landed must never be reported as
	// failed because a cosmetic step could not run (FR-013).
	if library.IsMusicKind(library.UploadKind(kind)) {
		d.tagUploadedTrack(r, kind, folder, seasonFolder, name, req)
	}

	// The final name is returned because it is not always the one that was sent:
	// Go's multipart reader bases the filename, so "a/b.mkv" arrives as "b.mkv".
	httpx.JSON(w, http.StatusOK, map[string]any{"destination": dest, "file": name, "uploaded": true})
}

// uploadParent resolves an upload kind to the folder SynoDL is configured to
// use. A client can name only a kind; it can never supply a path.
//
// The two music parents come from the operator config rather than from a
// download source (spec 1040): music has no source to inherit a parent from, and
// the worker's PVC claim name is not something the server can write to.
func (d Deps) uploadParent(kind string) (string, bool) {
	if library.IsMusicKind(library.UploadKind(kind)) {
		m, err := d.Store.GetMusicParents()
		if err != nil {
			return "", false
		}
		p := m.Music
		if kind == string(library.KindMusicVideo) {
			p = m.MusicVideo
		}
		p = strings.Trim(strings.TrimSpace(p), "/")
		return p, p != ""
	}
	providers, err := d.Store.ListProviders()
	if err != nil {
		return "", false
	}
	for _, p := range libraryParents(providers) {
		if kind == "tv" && p.TV {
			return p.Path, true
		}
		if kind == "movie" && p.Movies {
			return p.Path, true
		}
	}
	return "", false
}

// tagUploadedTrack starts the worker that writes the details into the file.
//
// Everything about this is deliberately quiet. There is no orchestrator on a
// Compose install and there may be none reachable on a cluster; there is no
// record to update and nothing to tell the user, because the upload has already
// succeeded and the file is already where it belongs. A failure here is a
// cosmetic loss, logged for an operator and invisible to everyone else.
//
// Artwork tags nothing: the cover IS the thing being embedded, and lyrics are a
// sidecar a media server reads from beside the file rather than from inside it.
func (d Deps) tagUploadedTrack(r *http.Request, kind, artistFolder, albumFolder, name string, req uploadRequest) {
	if d.Jobs == nil || !d.Cfg.YtdlConfigured() {
		return
	}
	if library.IsArtwork(name) || strings.EqualFold(path.Ext(name), ".lrc") {
		return
	}

	mode := ytdl.ModeMusic
	if kind == string(library.KindMusicVideo) {
		mode = ytdl.ModeMusicVideo
	}
	libs := d.ytdlLibraries()
	if _, ok := libs[mode]; !ok {
		return // no volume to mount, so nothing to tag
	}

	rel := path.Join(artistFolder, albumFolder, name)
	// The cover is named by the server, so this asks for it by the name it would
	// have been given rather than by looking: a worker that finds no cover simply
	// tags without one.
	cover := path.Join(artistFolder, albumFolder, "cover.jpg")

	job, err := ytdl.BuildTagJob(ytdl.JobConfig{
		Namespace: d.Cfg.YtdlNamespace,
		Image:     d.Cfg.YtdlImage,
		// Its own id, not a download's: an upload has no ytdl_downloads record
		// and must not collide with one that does.
		RequestID: newRequestID(),
		Mode:      mode,
		Libraries: libs,
	}, ytdl.TagRequest{
		RelPath:  rel,
		CoverRel: cover,
		Title:    req.Track,
		Artist:   req.Artist,
		Album:    req.Album,
	})
	if err != nil {
		log.Printf("upload tagging: could not build a worker: %v", err)
		return
	}
	if _, err := d.Jobs.CreateJob(r.Context(), job); err != nil && !k8s.IsConflict(err) {
		log.Printf("upload tagging: could not start a worker: %v", err)
	}
}
