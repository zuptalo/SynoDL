// Package synomock is a fake Synology DSM: just enough of the Web API for
// SynoDL to develop and e2e-test against without real hardware (constitution,
// "Mock-DSM dev parity"). It implements the same allowlisted APIs the real
// client speaks — SYNO.API.Info discovery, Auth (with an OTP account),
// DownloadStation Task/Statistic, FileStation List — plus /__mock/* control
// endpoints so tests can reset and seed deterministic fixtures.
//
// Fixed accounts: admin/secret, and otpuser/secret which requires OTP 000000.
package synomock

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

type account struct {
	Password string
	OTP      string // empty = no 2FA
	// FailCode, when non-zero, makes every login for this account fail with
	// that SYNO.API.Auth error even with the right password — reproducing the
	// DSM-side account states from the Login Web API Guide (disabled, blocked
	// IP, expired password) without real hardware (spec 1001 FR-004).
	FailCode int
}

// Task is the mock's task record and the /__mock/seed wire shape. Progress for
// downloading tasks is computed on read from Rate and elapsed time, so a live
// `make start` session shows moving numbers while rate-0 fixtures stay frozen
// for deterministic e2e assertions.
type Task struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Size        int64  `json:"size"`
	Downloaded  int64  `json:"downloaded"`
	Uploaded    int64  `json:"uploaded"`
	Rate        int64  `json:"rate"`   // bytes/sec while downloading
	UpRate      int64  `json:"upRate"` // bytes/sec while seeding
	Peers       int    `json:"peers"`
	Seeders     int    `json:"seeders"`
	CreatedAt   int64  `json:"createdAt"` // unix seconds; 0 = now
	Destination string `json:"destination"`
	URI         string `json:"uri"`         // source URL/magnet
	ErrorDetail string `json:"errorDetail"` // DSM status_extra.error_detail keyword, e.g. "broken_link"

	resumedAt      time.Time
	baseDownloaded int64
}

type Server struct {
	mu       sync.Mutex
	accounts map[string]account
	sessions map[string]string // sid -> account name
	tasks    []*Task
	nextID   int
	nextSID  int
	offset   time.Duration // virtual-clock offset advanced by /__mock/tick
	folders  map[string][]string
	// Files placed by SYNO.FileStation.Upload, keyed by folder (spec 1022), so a
	// test can assert what actually landed and where.
	uploads map[string][]string
	// Fake download sources (spec 0007), so dev and e2e can exercise the catalog
	// without pasting real credentials for a real site. See sources.go.
	zarSrc *SourceState
	tnSrc  *SourceState
}

func New() *Server {
	s := &Server{zarSrc: newSourceState("zar"), tnSrc: newSourceState("tn")}
	s.resetLocked()
	return s
}

func (s *Server) now() time.Time { return time.Now().Add(s.offset) }

// resetLocked restores the default fixture state. Caller holds s.mu (or is the
// constructor).
func (s *Server) resetLocked() {
	s.accounts = map[string]account{
		"admin":    {Password: "secret"},
		"otpuser":  {Password: "secret", OTP: "000000"},
		"disabled": {Password: "secret", FailCode: 401},
		"blocked":  {Password: "secret", FailCode: 407},
		"expired":  {Password: "secret", FailCode: 409},
	}
	s.sessions = map[string]string{}
	s.nextID = 0
	s.nextSID = 0
	s.offset = 0
	s.resetFoldersLocked()
	s.uploads = map[string][]string{}
	s.tasks = nil
	s.seedLocked([]Task{
		{Name: "ubuntu-24.04-desktop-amd64.iso", Type: "bt", Status: "downloading",
			Size: 6_114_656_256, Downloaded: 1_528_664_064, Rate: 8_500_000,
			Peers: 42, Seeders: 118, Destination: "home/Downloads"},
		{Name: "debian-13.1.0-arm64-netinst.iso", Type: "http", Status: "finished",
			Size: 702_545_920, Downloaded: 702_545_920, Destination: "home/Downloads"},
		{Name: "big-buck-bunny-4k.mkv", Type: "bt", Status: "paused",
			Size: 8_388_608_000, Downloaded: 4_194_304_000, Peers: 3, Seeders: 9,
			Destination: "movie/4K"},
	})
}

// seedLocked replaces the task list with the given fixtures, assigning ids and
// timestamps. Caller holds s.mu.
func (s *Server) seedLocked(tasks []Task) {
	s.tasks = nil
	for i := range tasks {
		t := tasks[i]
		s.nextID++
		t.ID = fmt.Sprintf("dbid_%03d", s.nextID)
		if t.CreatedAt == 0 {
			t.CreatedAt = s.now().Unix()
		}
		t.resumedAt = s.now()
		t.baseDownloaded = t.Downloaded
		s.tasks = append(s.tasks, &t)
	}
}

// Seed replaces the task list with the given fixtures. Exported for Go tests
// (the syno client contract tests) that need a deterministic task — e.g. an
// errored one carrying an error_detail — without going through /__mock/seed.
func (s *Server) Seed(tasks []Task) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seedLocked(tasks)
}

// advanceLocked folds elapsed virtual time into each downloading task.
func (s *Server) advanceLocked() {
	now := s.now()
	for _, t := range s.tasks {
		if t.Status != "downloading" || t.Rate <= 0 {
			continue
		}
		t.Downloaded = t.baseDownloaded + int64(now.Sub(t.resumedAt).Seconds())*t.Rate
		if t.Downloaded >= t.Size {
			t.Downloaded = t.Size
			t.Status = "finished"
		}
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/webapi/query.cgi", s.handleInfo)
	mux.HandleFunc("/webapi/auth.cgi", s.handleAuth)
	mux.HandleFunc("/webapi/DownloadStation/task.cgi", s.handleTask)
	mux.HandleFunc("/webapi/DownloadStation/statistic.cgi", s.handleStatistic)
	mux.HandleFunc("/webapi/entry.cgi", s.handleFileStation)
	s.registerSources(mux)
	mux.HandleFunc("POST /__mock/reset", s.handleReset)
	mux.HandleFunc("POST /__mock/seed", s.handleSeed)
	mux.HandleFunc("POST /__mock/tick", s.handleTick)
	mux.HandleFunc("POST /__mock/library", s.handleLibrary)
	return mux
}

func ok(w http.ResponseWriter, data any) {
	resp := map[string]any{"success": true}
	if data != nil {
		resp["data"] = data
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func fail(w http.ResponseWriter, code int) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": false,
		"error":   map[string]int{"code": code},
	})
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	// DSM7-shaped discovery table: FileStation rides entry.cgi, Download
	// Station keeps its classic per-app cgi paths.
	ok(w, map[string]any{
		"SYNO.API.Auth":                  map[string]any{"path": "auth.cgi", "minVersion": 1, "maxVersion": 7},
		"SYNO.DownloadStation.Task":      map[string]any{"path": "DownloadStation/task.cgi", "minVersion": 1, "maxVersion": 3},
		"SYNO.DownloadStation.Statistic": map[string]any{"path": "DownloadStation/statistic.cgi", "minVersion": 1, "maxVersion": 1},
		"SYNO.FileStation.List":          map[string]any{"path": "entry.cgi", "minVersion": 1, "maxVersion": 2},
		"SYNO.FileStation.CreateFolder":  map[string]any{"path": "entry.cgi", "minVersion": 1, "maxVersion": 2},
		"SYNO.FileStation.Upload":        map[string]any{"path": "entry.cgi", "minVersion": 1, "maxVersion": 2},
	})
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil && err != http.ErrNotMultipart {
		fail(w, 101)
		return
	}
	switch r.FormValue("method") {
	case "login":
		s.mu.Lock()
		defer s.mu.Unlock()
		acct, okAcct := s.accounts[r.FormValue("account")]
		if !okAcct || acct.Password != r.FormValue("passwd") {
			fail(w, 400)
			return
		}
		if acct.FailCode != 0 {
			fail(w, acct.FailCode)
			return
		}
		if acct.OTP != "" {
			switch r.FormValue("otp_code") {
			case acct.OTP:
			case "":
				fail(w, 403)
				return
			default:
				fail(w, 404)
				return
			}
		}
		s.nextSID++
		sid := fmt.Sprintf("mock-sid-%d", s.nextSID)
		s.sessions[sid] = r.FormValue("account")
		ok(w, map[string]string{"sid": sid})
	case "logout":
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.sessions, r.FormValue("_sid"))
		ok(w, nil)
	default:
		fail(w, 101)
	}
}

// authed reports whether the request carries a live sid (form or query).
func (s *Server) authedLocked(r *http.Request) bool {
	_, live := s.sessions[r.FormValue("_sid")]
	return live
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	// Task-file uploads arrive as multipart; everything else as form/query.
	if err := r.ParseMultipartForm(64 << 20); err != nil && err != http.ErrNotMultipart {
		fail(w, 101)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authedLocked(r) {
		fail(w, 106)
		return
	}
	switch r.FormValue("method") {
	case "list":
		s.advanceLocked()
		tasks := make([]map[string]any, 0, len(s.tasks))
		for _, t := range s.tasks {
			dl, ul := int64(0), int64(0)
			if t.Status == "downloading" {
				dl = t.Rate
			}
			if t.Status == "seeding" || t.Status == "downloading" {
				ul = t.UpRate
			}
			entry := map[string]any{
				"id": t.ID, "title": t.Name, "type": t.Type, "status": t.Status,
				"size": t.Size, "username": "admin",
				"additional": map[string]any{
					"detail": map[string]any{
						"create_time": t.CreatedAt, "destination": t.Destination, "uri": t.URI,
						"connected_peers": t.Peers, "connected_seeders": t.Seeders,
					},
					"transfer": map[string]any{
						"size_downloaded": t.Downloaded, "size_uploaded": t.Uploaded,
						"speed_download": dl, "speed_upload": ul,
					},
				},
			}
			// DSM reports a failed task's cause in status_extra.error_detail; only
			// present when set, mirroring the real NAS.
			if t.ErrorDetail != "" {
				entry["status_extra"] = map[string]any{"error_detail": t.ErrorDetail}
			}
			tasks = append(tasks, entry)
		}
		ok(w, map[string]any{"tasks": tasks, "total": len(tasks)})
	case "create":
		type mk struct{ name, uri string }
		var items []mk
		if uri := r.FormValue("uri"); uri != "" {
			for _, u := range strings.Split(uri, ",") {
				items = append(items, mk{taskNameFromURI(u), strings.TrimSpace(u)})
			}
		}
		if r.MultipartForm != nil && len(r.MultipartForm.File["file"]) > 0 {
			items = append(items, mk{r.MultipartForm.File["file"][0].Filename, ""})
		}
		if len(items) == 0 {
			fail(w, 101)
			return
		}
		for _, it := range items {
			s.nextID++
			s.tasks = append(s.tasks, &Task{
				ID: fmt.Sprintf("dbid_%03d", s.nextID), Name: it.name, Type: "bt",
				Status: "downloading", Size: 1_073_741_824, Rate: 25_000_000,
				Peers: 5, Seeders: 20, CreatedAt: s.now().Unix(),
				Destination: r.FormValue("destination"), URI: it.uri,
				resumedAt: s.now(),
			})
		}
		ok(w, nil)
	case "pause", "resume", "delete":
		method := r.FormValue("method")
		wanted := map[string]bool{}
		for _, id := range strings.Split(r.FormValue("id"), ",") {
			wanted[strings.TrimSpace(id)] = true
		}
		s.advanceLocked()
		kept := s.tasks[:0]
		for _, t := range s.tasks {
			if !wanted[t.ID] {
				kept = append(kept, t)
				continue
			}
			switch method {
			case "pause":
				if t.Status == "downloading" {
					t.baseDownloaded = t.Downloaded
					t.Status = "paused"
				}
				kept = append(kept, t)
			case "resume":
				if t.Status == "paused" {
					t.baseDownloaded = t.Downloaded
					t.resumedAt = s.now()
					t.Status = "downloading"
				}
				kept = append(kept, t)
			case "delete":
				// dropped
			}
		}
		s.tasks = kept
		ok(w, nil)
	default:
		fail(w, 101)
	}
}

func (s *Server) handleStatistic(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authedLocked(r) {
		fail(w, 106)
		return
	}
	s.advanceLocked()
	var dl, ul int64
	for _, t := range s.tasks {
		if t.Status == "downloading" {
			dl += t.Rate
			ul += t.UpRate
		}
		if t.Status == "seeding" {
			ul += t.UpRate
		}
	}
	ok(w, map[string]int64{"speed_download": dl, "speed_upload": ul})
}

func (s *Server) handleFileStation(w http.ResponseWriter, r *http.Request) {
	// An upload arrives as multipart, which ParseForm alone does not read.
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		_ = r.ParseMultipartForm(32 << 20)
	}
	_ = r.ParseForm()
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authedLocked(r) {
		fail(w, 106)
		return
	}
	switch r.FormValue("api") {
	case "SYNO.FileStation.List":
		switch r.FormValue("method") {
		case "list_share":
			shares := make([]map[string]any, 0)
			for _, name := range s.folders[""] {
				shares = append(shares, map[string]any{"isdir": true, "name": name, "path": "/" + name})
			}
			ok(w, map[string]any{"shares": shares, "total": len(shares)})
		case "list":
			dir := r.FormValue("folder_path")
			children, exists := s.folders[dir]
			if !exists {
				fail(w, 408) // FileStation: no such file or directory
				return
			}
			files := make([]map[string]any, 0)
			for _, name := range children {
				files = append(files, map[string]any{"isdir": true, "name": name, "path": path.Join(dir, name)})
			}
			ok(w, map[string]any{"files": files, "total": len(files)})
		default:
			fail(w, 101)
		}
	case "SYNO.FileStation.Upload":
		if r.FormValue("method") != "upload" {
			fail(w, 101)
			return
		}
		dir := strings.TrimRight(r.FormValue("path"), "/")
		if _, exists := s.folders[dir]; !exists {
			fail(w, 408) // no such folder
			return
		}
		_, hdr, err := r.FormFile("file")
		if err != nil || hdr == nil || hdr.Filename == "" {
			fail(w, 101)
			return
		}
		for _, existing := range s.uploads[dir] {
			if existing == hdr.Filename {
				// Real DSM refuses when overwrite is not requested, and the
				// client deliberately never requests it.
				fail(w, 414)
				return
			}
		}
		s.uploads[dir] = append(s.uploads[dir], hdr.Filename)
		ok(w, map[string]any{"blSkip": false})
		return
	case "SYNO.FileStation.CreateFolder":
		if r.FormValue("method") != "create" {
			fail(w, 101)
			return
		}
		parent := r.FormValue("folder_path")
		name := r.FormValue("name")
		if parent == "" || name == "" {
			fail(w, 101)
			return
		}
		if _, exists := s.folders[parent]; !exists {
			fail(w, 408) // parent must exist (force_parent not set)
			return
		}
		full := path.Join(parent, name)
		s.folders[parent] = append(s.folders[parent], name)
		if _, exists := s.folders[full]; !exists {
			s.folders[full] = []string{}
		}
		ok(w, map[string]any{"folders": []map[string]any{{"isdir": true, "name": name, "path": full}}})
	default:
		fail(w, 103)
	}
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.resetLocked()
	ok(w, nil)
}

func (s *Server) handleSeed(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Tasks []Task `json:"tasks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, 101)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seedLocked(body.Tasks)
	ok(w, nil)
}

// handleLibrary seeds folders into the tree, so a test can set up "the NAS
// already has these titles" (spec 0008). The fixture tree in resetLocked is
// hardcoded and there was previously no way to add to it, which made the
// ownership markers untestable end to end.
//
// Parents are created implicitly, so one call can seed a whole path. Seeding is
// additive and idempotent per name; POST /__mock/reset restores the fixtures.
func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	var body struct {
		// Reset restores the fixture tree before seeding, so a test starts from a
		// known library without also resetting sessions, accounts, and tasks the
		// way POST /__mock/reset does.
		Reset   bool                `json:"reset"`
		Folders map[string][]string `json:"folders"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, 101)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if body.Reset {
		s.resetFoldersLocked()
	}
	for dir, names := range body.Folders {
		s.ensureDirLocked(dir)
		for _, name := range names {
			s.addChildLocked(dir, name)
		}
	}
	ok(w, nil)
}

// resetFoldersLocked restores the fixture folder tree, which mirrors the
// reference screenshots' shares. Split out of resetLocked so a test can clear
// seeded library folders without also dropping its session (spec 0008).
func (s *Server) resetFoldersLocked() {
	s.folders = map[string][]string{
		"":         {"home", "movie", "music", "music-video", "rated-video", "tv-show"},
		"/tv-show": {"Friends", "The Wire"},
		"/movie":   {"4K", "Kids"},
		"/music":   {},
		"/home":    {"Downloads"},
	}
}

// ensureDirLocked makes dir (and every ancestor) exist in the tree, so a caller
// can seed "/tv-show/Friends 1994/Season 1" without first creating each level.
func (s *Server) ensureDirLocked(dir string) {
	dir = strings.TrimRight(dir, "/")
	if dir == "" {
		return
	}
	if _, exists := s.folders[dir]; exists {
		return
	}
	parent := path.Dir(dir)
	if parent == "." || parent == "/" {
		parent = ""
	}
	s.ensureDirLocked(parent)
	s.addChildLocked(parent, path.Base(dir))
}

// addChildLocked adds name under dir if it is not already there, and gives the
// child its own (empty) entry so it can be listed and descended into.
func (s *Server) addChildLocked(dir, name string) {
	if name == "" {
		return
	}
	for _, existing := range s.folders[dir] {
		if existing == name {
			return
		}
	}
	s.folders[dir] = append(s.folders[dir], name)
	full := path.Join(dir, name)
	if dir == "" {
		full = "/" + name
	}
	if _, exists := s.folders[full]; !exists {
		s.folders[full] = []string{}
	}
}

func (s *Server) handleTick(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Seconds int `json:"seconds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, 101)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.offset += time.Duration(body.Seconds) * time.Second
	ok(w, nil)
}

// taskNameFromURI derives a display name the way DSM does: the URL's last path
// segment, or the magnet dn= parameter when present.
func taskNameFromURI(uri string) string {
	uri = strings.TrimSpace(uri)
	if strings.HasPrefix(uri, "magnet:") {
		for _, kv := range strings.Split(strings.TrimPrefix(uri, "magnet:?"), "&") {
			if v, found := strings.CutPrefix(kv, "dn="); found {
				if dec, err := url.QueryUnescape(v); err == nil {
					return dec
				}
				return v
			}
		}
		return "magnet download"
	}
	// Real DSM names an HTTP task after the URL's FILE, so the query string is
	// not part of the name. That matters beyond cosmetics: signed download links
	// carry credentials and account identifiers in their query
	// (?md5=…&u=…&expires=…), and a mock that echoed those into a task name would
	// put them on screen and into any test that asserts on names.
	if i := strings.IndexAny(uri, "?#"); i >= 0 {
		uri = uri[:i]
	}
	trimmed := strings.TrimRight(uri, "/")
	if i := strings.LastIndexByte(trimmed, '/'); i >= 0 && i < len(trimmed)-1 {
		return trimmed[i+1:]
	}
	return uri
}
