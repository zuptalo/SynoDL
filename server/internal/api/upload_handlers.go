package api

import (
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"synodl/server/internal/httpx"
	"synodl/server/internal/library"
	"synodl/server/internal/store"
	"synodl/server/internal/syno"
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
				d.streamUploadedFile(w, r, u, kind, title, season, size, overwrite, part)
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
			}
		}
	})
}

// streamUploadedFile validates everything, makes the folders, and pipes the part
// to the NAS. Nothing here is buffered.
func (d Deps) streamUploadedFile(
	w http.ResponseWriter, r *http.Request, u *store.User,
	kind, title, season, size, overwrite string, part *multipart.Part,
) {
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
	// general write-anything-to-the-NAS capability.
	if !library.AllowedUploadType(name) {
		httpx.Error(w, http.StatusUnsupportedMediaType,
			"only video, subtitle, artwork and .nfo files can be uploaded")
		return
	}

	parent, ok := d.uploadParent(kind)
	if !ok {
		httpx.JSON(w, http.StatusConflict, map[string]any{"error": "parent_unset"})
		return
	}
	// The same naming a download produces, so an uploaded title and a downloaded
	// one are indistinguishable afterwards.
	folder := sanitizeFolderName(library.PlexName(title))
	if !validFolderName(folder) {
		httpx.Error(w, http.StatusBadRequest, "a title is required")
		return
	}
	dest := parent + "/" + folder
	seasonFolder := ""
	if n, err := strconv.Atoi(strings.TrimSpace(season)); err == nil && n >= 0 {
		seasonFolder = sanitizeFolderName(library.SeasonFolder(n))
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
	// The final name is returned because it is not always the one that was sent:
	// Go's multipart reader bases the filename, so "a/b.mkv" arrives as "b.mkv".
	httpx.JSON(w, http.StatusOK, map[string]any{"destination": dest, "file": name, "uploaded": true})
}

// uploadParent resolves "movie" or "tv" to the folder SynoDL is configured to
// use. A client can name only these two; it can never supply a path.
func (d Deps) uploadParent(kind string) (string, bool) {
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
