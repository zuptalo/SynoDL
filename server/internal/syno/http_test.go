package syno

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"synodl/server/internal/synomock"
)

// newTestClient boots the in-repo mock DSM behind httptest and points a real
// HTTPClient at it — the same pairing production uses in dev, so these tests
// double as a contract check between the client and the mock.
func newTestClient(t *testing.T) *HTTPClient {
	t.Helper()
	srv := httptest.NewServer(synomock.New().Handler())
	t.Cleanup(srv.Close)
	return NewHTTPClient(srv.URL, false)
}

func login(t *testing.T, c *HTTPClient) string {
	t.Helper()
	sid, err := c.Login(context.Background(), "admin", "secret", "")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if sid == "" {
		t.Fatal("Login returned empty sid")
	}
	return sid
}

func TestLoginSuccessAndLogout(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)
	if err := c.Logout(context.Background(), sid); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	// The sid is dead now; a task list with it must classify as session-expired.
	_, err := c.ListTasks(context.Background(), sid)
	se := AsError(err)
	if se == nil || se.Kind != KindSession {
		t.Fatalf("ListTasks after logout: got %v, want KindSession", err)
	}
}

func TestLoginErrorMapping(t *testing.T) {
	c := newTestClient(t)
	cases := []struct {
		name, account, password, otp string
		want                         Kind
	}{
		{"wrong password", "admin", "nope", "", KindCredentials},
		{"unknown account", "ghost", "secret", "", KindCredentials},
		{"otp missing", "otpuser", "secret", "", KindOTPRequired},
		{"otp wrong", "otpuser", "secret", "123456", KindOTPInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.Login(context.Background(), tc.account, tc.password, tc.otp)
			se := AsError(err)
			if se == nil || se.Kind != tc.want {
				t.Fatalf("got %v, want kind %s", err, tc.want)
			}
		})
	}
	// And the OTP path succeeds with the right code.
	if _, err := c.Login(context.Background(), "otpuser", "secret", "000000"); err != nil {
		t.Fatalf("Login with correct OTP: %v", err)
	}
}

func TestLoginGuideAccountStates(t *testing.T) {
	// End-to-end through the real client: the mock's guide-state accounts
	// (disabled/blocked/expired) classify to their dedicated kinds (spec 1001).
	c := newTestClient(t)
	cases := []struct {
		account string
		want    Kind
	}{
		{"disabled", KindAccountDisabled},
		{"blocked", KindIPBlocked},
		{"expired", KindPasswordExpired},
	}
	for _, tc := range cases {
		_, err := c.Login(context.Background(), tc.account, "secret", "")
		se := AsError(err)
		if se == nil || se.Kind != tc.want {
			t.Errorf("%s: got %v, want kind %s", tc.account, err, tc.want)
		}
	}
}

func TestListTasksFlattensAdditional(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)
	tasks, err := c.ListTasks(context.Background(), sid)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("got %d tasks, want the 3 default fixtures", len(tasks))
	}
	dl := tasks[0]
	if dl.Name != "ubuntu-24.04-desktop-amd64.iso" || dl.Status != "downloading" {
		t.Errorf("task[0] = %+v, want the downloading ubuntu fixture", dl)
	}
	if dl.Size == 0 || dl.Downloaded == 0 || dl.DownloadSpeed == 0 {
		t.Errorf("transfer fields not flattened: %+v", dl)
	}
	if dl.Peers == 0 || dl.Seeders == 0 || dl.CreatedAt == 0 || dl.Destination == "" {
		t.Errorf("detail fields not flattened: %+v", dl)
	}
}

func TestListTasksSurfacesErrorDetail(t *testing.T) {
	mock := synomock.New()
	srv := httptest.NewServer(mock.Handler())
	t.Cleanup(srv.Close)
	c := NewHTTPClient(srv.URL, false)
	mock.Seed([]synomock.Task{
		{Name: "broken.iso", Type: "bt", Status: "error", ErrorDetail: "broken_link"},
		{Name: "ok.iso", Type: "http", Status: "finished", Size: 100, Downloaded: 100},
	})
	sid := login(t, c)

	tasks, err := c.ListTasks(context.Background(), sid)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("got %d tasks, want 2", len(tasks))
	}
	if tasks[0].Status != "error" || tasks[0].ErrorDetail != "broken_link" {
		t.Errorf("errored task = %+v, want ErrorDetail=broken_link", tasks[0])
	}
	if tasks[1].ErrorDetail != "" {
		t.Errorf("non-errored task carried ErrorDetail=%q, want empty", tasks[1].ErrorDetail)
	}
}

func TestCreatePauseResumeDeleteRoundTrip(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)
	ctx := context.Background()

	if err := c.CreateTaskURIs(ctx, sid, []string{"http://example.com/file.iso"},
		CreateOpts{Destination: "movie"}); err != nil {
		t.Fatalf("CreateTaskURIs: %v", err)
	}
	tasks, err := c.ListTasks(ctx, sid)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	var created *Task
	for i := range tasks {
		if tasks[i].Name == "file.iso" {
			created = &tasks[i]
		}
	}
	if created == nil {
		t.Fatalf("created task not in list: %+v", tasks)
	}
	if created.Destination != "movie" {
		t.Errorf("destination = %q, want movie", created.Destination)
	}

	if err := c.PauseTasks(ctx, sid, []string{created.ID}); err != nil {
		t.Fatalf("PauseTasks: %v", err)
	}
	if got := statusOf(t, c, sid, created.ID); got != "paused" {
		t.Errorf("after pause: status %q", got)
	}
	if err := c.ResumeTasks(ctx, sid, []string{created.ID}); err != nil {
		t.Fatalf("ResumeTasks: %v", err)
	}
	if got := statusOf(t, c, sid, created.ID); got != "downloading" {
		t.Errorf("after resume: status %q", got)
	}
	if err := c.DeleteTasks(ctx, sid, []string{created.ID}); err != nil {
		t.Fatalf("DeleteTasks: %v", err)
	}
	if got := statusOf(t, c, sid, created.ID); got != "" {
		t.Errorf("after delete: task still present with status %q", got)
	}
}

func statusOf(t *testing.T, c *HTTPClient, sid, id string) string {
	t.Helper()
	tasks, err := c.ListTasks(context.Background(), sid)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	for _, task := range tasks {
		if task.ID == id {
			return task.Status
		}
	}
	return ""
}

func TestCreateTaskFileMultipart(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)
	err := c.CreateTaskFile(context.Background(), sid, "linux.torrent",
		strings.NewReader("d8:announce0:e"), CreateOpts{Destination: "tv-show"})
	if err != nil {
		t.Fatalf("CreateTaskFile: %v", err)
	}
	tasks, _ := c.ListTasks(context.Background(), sid)
	found := false
	for _, task := range tasks {
		if task.Name == "linux.torrent" && task.Destination == "tv-show" {
			found = true
		}
	}
	if !found {
		t.Fatalf("uploaded torrent task not found in %+v", tasks)
	}
}

func TestStats(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)
	stats, err := c.Stats(context.Background(), sid)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.DownloadSpeed == 0 {
		t.Error("DownloadSpeed = 0, want the downloading fixture's rate")
	}
}

func TestFileStationBrowse(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)
	shares, err := c.ListShares(context.Background(), sid)
	if err != nil {
		t.Fatalf("ListShares: %v", err)
	}
	if len(shares) == 0 {
		t.Fatal("no shares returned")
	}
	var tvShow *Folder
	for i := range shares {
		if shares[i].Name == "tv-show" {
			tvShow = &shares[i]
		}
	}
	if tvShow == nil || tvShow.Path != "/tv-show" {
		t.Fatalf("tv-show share missing or mispathed: %+v", shares)
	}
	sub, err := c.ListFolder(context.Background(), sid, tvShow.Path)
	if err != nil {
		t.Fatalf("ListFolder: %v", err)
	}
	if len(sub) == 0 || sub[0].Path != "/tv-show/Friends" {
		t.Fatalf("subfolders = %+v, want /tv-show/Friends first", sub)
	}
	// Unknown paths surface as a NAS error, not a silent empty list.
	if _, err := c.ListFolder(context.Background(), sid, "/nope"); AsError(err) == nil {
		t.Fatalf("ListFolder(/nope) = %v, want *Error", err)
	}
}

func TestFileStationCreateFolder(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)

	folder, err := c.CreateFolder(context.Background(), sid, "/movie", "Docs")
	if err != nil {
		t.Fatalf("CreateFolder: %v", err)
	}
	if folder.Name != "Docs" || folder.Path != "/movie/Docs" {
		t.Fatalf("created folder = %+v", folder)
	}
	// The new folder is now browsable under its parent.
	sub, err := c.ListFolder(context.Background(), sid, "/movie")
	if err != nil {
		t.Fatalf("ListFolder: %v", err)
	}
	found := false
	for _, f := range sub {
		if f.Name == "Docs" {
			found = true
		}
	}
	if !found {
		t.Fatalf("created folder not listed under /movie: %+v", sub)
	}
	// Creating under a missing parent surfaces a NAS error.
	if _, err := c.CreateFolder(context.Background(), sid, "/nope", "x"); AsError(err) == nil {
		t.Fatalf("CreateFolder under missing parent should error")
	}
}

// Uploading several files into one new title folder creates the folder once and
// then re-creates it per file, so the second create has to behave the way a real
// DSM does — refuse, rather than quietly add a second entry under the parent.
// When the mock appended unconditionally the title appeared twice under /movie,
// which is exactly what the ownership index reads.
func TestCreateFolderTwiceDoesNotDuplicate(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)

	if _, err := c.CreateFolder(context.Background(), sid, "/movie", "Dupe"); err != nil {
		t.Fatalf("first CreateFolder: %v", err)
	}
	// Real DSM answers "already exists"; the caller (ensureSubfolder) treats a
	// failed create as "maybe it is there" and reuses it.
	if _, err := c.CreateFolder(context.Background(), sid, "/movie", "Dupe"); AsError(err) == nil {
		t.Fatal("re-creating an existing folder should surface a NAS error")
	}

	sub, err := c.ListFolder(context.Background(), sid, "/movie")
	if err != nil {
		t.Fatalf("ListFolder: %v", err)
	}
	n := 0
	for _, f := range sub {
		if f.Name == "Dupe" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("/movie lists %q %d times, want exactly 1: %+v", "Dupe", n, sub)
	}
}

func TestUnreachableNAS(t *testing.T) {
	// A dead port: connection refused must map to KindUnreachable, never panic.
	c := NewHTTPClient("http://127.0.0.1:1", false)
	_, err := c.Login(context.Background(), "admin", "secret", "")
	se := AsError(err)
	if se == nil || se.Kind != KindUnreachable {
		t.Fatalf("got %v, want KindUnreachable", err)
	}
}

func TestDiscoveryFailureRetries(t *testing.T) {
	// First request hits a server that 500s discovery; after it starts
	// answering, the same client must recover (the api table isn't poisoned).
	broken := true
	mock := synomock.New().Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if broken {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		mock.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	c := NewHTTPClient(srv.URL, false)
	if _, err := c.Login(context.Background(), "admin", "secret", ""); AsError(err) == nil {
		t.Fatal("expected error while discovery is broken")
	}
	broken = false
	if _, err := c.Login(context.Background(), "admin", "secret", ""); err != nil {
		t.Fatalf("Login after recovery: %v", err)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		api  string
		code int
		want Kind
	}{
		// Per the DSM Login Web API Guide's SYNO.API.Auth error table.
		{apiAuth, 400, KindCredentials},
		{apiAuth, 401, KindAccountDisabled},
		{apiAuth, 402, KindPermission},
		{apiAuth, 403, KindOTPRequired},
		{apiAuth, 404, KindOTPInvalid},
		{apiAuth, 406, KindOTPRequired}, // "enforce 2FA" needs the same user action as 403
		{apiAuth, 407, KindIPBlocked},
		{apiAuth, 408, KindPasswordExpired},
		{apiAuth, 409, KindPasswordExpired},
		{apiAuth, 410, KindPasswordExpired},
		{apiAuth, 105, KindPermission},
		{apiTask, 105, KindSession},
		{apiTask, 106, KindSession},
		{apiTask, 107, KindSession},
		{apiTask, 119, KindSession},
		{apiTask, 544, KindNAS},
		{apiFSList, 408, KindNAS},
	}
	for _, tc := range cases {
		if got := classify(tc.api, tc.code); got != tc.want {
			t.Errorf("classify(%s, %d) = %s, want %s", tc.api, tc.code, got, tc.want)
		}
	}
}

// Spec 1022: the one call that writes file CONTENT to the NAS. Exercised
// against synomock so the multipart shape, the API discovery, and the
// no-overwrite behaviour are all verified against something that answers like
// DSM rather than against a hand-rolled expectation.
func TestFileStationUpload(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)

	const film = "pretend this is a film"
	if err := c.UploadFile(context.Background(), sid, "/movie", "Dune (2021).mkv", int64(len(film)), false,
		strings.NewReader(film)); err != nil {
		t.Fatalf("upload: %v", err)
	}

	// A second upload of the same name must FAIL rather than overwrite. The
	// client never sends overwrite=true, so DSM refuses — which is what lets a
	// collision be reported instead of silently replacing somebody's file.
	const other = "a different film"
	err := c.UploadFile(context.Background(), sid, "/movie", "Dune (2021).mkv", int64(len(other)), false,
		strings.NewReader(other))
	if err == nil {
		t.Fatal("re-uploading the same name should fail, not overwrite")
	}

	// A folder that does not exist is an error, not a silent create.
	if err := c.UploadFile(context.Background(), sid, "/movie/Nope", "x.mkv", 1, false,
		strings.NewReader("x")); err == nil {
		t.Error("uploading into a missing folder should fail")
	}
}

// The body must stream: a large file may not be assembled in memory first.
func TestUploadStreamsRatherThanBuffers(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)

	// A reader that would be ruinous to buffer, counted as it is consumed.
	const size = 8 << 20
	read := 0
	body := &countingReader{n: size, seen: &read}
	if err := c.UploadFile(context.Background(), sid, "/movie", "big.mkv", int64(size), false, body); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if read != size {
		t.Errorf("streamed %d bytes, want %d", read, size)
	}
}

// countingReader yields n zero bytes and records how many were consumed.
type countingReader struct {
	n    int
	seen *int
}

func (r *countingReader) Read(p []byte) (int, error) {
	if r.n == 0 {
		return 0, io.EOF
	}
	k := len(p)
	if k > r.n {
		k = r.n
	}
	for i := range p[:k] {
		p[i] = 0
	}
	r.n -= k
	*r.seen += k
	return k, nil
}

// The bug this pins: an upload sent with a chunked request body.
//
// Streaming the multipart through an io.Pipe leaves the length unknown, so Go
// sends Transfer-Encoding: chunked — and DSM's entry.cgi refuses an upload in
// that form, instantly, before reading any of the file. Every real upload failed
// while every test passed, because Go's own HTTP server (which synomock is)
// accepts chunked happily. So asserting "the upload succeeded" can never catch
// this; the wire framing itself has to be asserted.
func TestUploadDeclaresContentLengthAndIsNotChunked(t *testing.T) {
	var gotLen int64 = -1
	var gotTE []string
	inner := synomock.New().Handler()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The upload is the only multipart request the client makes.
		if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
			gotLen = r.ContentLength
			gotTE = append([]string(nil), r.TransferEncoding...)
		}
		inner.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	c := NewHTTPClient(srv.URL, false)
	sid := login(t, c)

	const film = "pretend this is a film"
	if err := c.UploadFile(context.Background(), sid, "/movie", "Dune (2021).mkv",
		int64(len(film)), false, strings.NewReader(film)); err != nil {
		t.Fatalf("upload: %v", err)
	}

	if gotLen <= 0 {
		t.Errorf("upload sent Content-Length %d; DSM requires a real length", gotLen)
	}
	for _, te := range gotTE {
		if te == "chunked" {
			t.Error("upload was sent chunked; DSM rejects a chunked upload body")
		}
	}
	// The declared length must cover the envelope as well as the file, or the
	// server would be left waiting for bytes that never come.
	if gotLen <= int64(len(film)) {
		t.Errorf("Content-Length %d does not exceed the %d-byte file, so the "+
			"multipart envelope was not counted", gotLen, len(film))
	}
}

// Every API the client can call must also be one it ASKS the NAS about.
//
// The two lists were separate — maxSupported (versions we negotiate) and a
// hand-written query string in endpointFor — and they silently disagreed:
// SYNO.FileStation.Upload was in the first and missing from the second. Real DSM
// answers only for the APIs named in the query, so the upload endpoint was never
// in the discovered table and endpointFor failed before making any request,
// returning a code-less error. Every upload on real hardware failed for a whole
// release while the suite stayed green, because the mock ignored the filter.
//
// This asserts the invariant directly, so a newly added API cannot be
// half-registered again.
func TestEveryCallableAPIIsDiscoverable(t *testing.T) {
	c := newTestClient(t)
	for api := range maxSupported {
		ep, err := c.endpointFor(context.Background(), api)
		if err != nil {
			t.Errorf("endpointFor(%s): %v — is it missing from the discovery query?", api, err)
			continue
		}
		if ep.Path == "" {
			t.Errorf("endpointFor(%s) returned an empty path", api)
		}
	}
}

// The discovery query is what the NAS is asked about, so it has to name every
// API the client negotiates a version for — checked without a server so the
// failure points at the list rather than at a request.
func TestDiscoveryQueryCoversMaxSupported(t *testing.T) {
	inQuery := map[string]bool{}
	for _, api := range discoverableAPIs() {
		inQuery[api] = true
	}
	for api := range maxSupported {
		if !inQuery[api] {
			t.Errorf("%s is callable but absent from the discovery query", api)
		}
	}
}

// Replacing is opt-in, and exists for one reason: an upload cut off part-way
// leaves a PARTIAL file on the NAS, and without overwrite the retry is refused
// with "already exists" — reporting a name collision for a file that is really a
// broken fragment. Never the default, because it destroys content.
func TestUploadOverwriteOnlyWhenAsked(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)
	const first, second = "a partial file", "the whole film"

	if err := c.UploadFile(context.Background(), sid, "/movie", "Dune (2021).mkv",
		int64(len(first)), false, strings.NewReader(first)); err != nil {
		t.Fatalf("first upload: %v", err)
	}
	// Without overwrite the name is defended.
	err := c.UploadFile(context.Background(), sid, "/movie", "Dune (2021).mkv",
		int64(len(second)), false, strings.NewReader(second))
	if se := AsError(err); se == nil || se.Code != 414 {
		t.Fatalf("re-upload without overwrite = %v, want DSM 414", err)
	}
	// With it, the retry succeeds and the fragment is replaced.
	if err := c.UploadFile(context.Background(), sid, "/movie", "Dune (2021).mkv",
		int64(len(second)), true, strings.NewReader(second)); err != nil {
		t.Fatalf("re-upload with overwrite: %v", err)
	}
}

// Ownership needs the FILES in a folder, not its subfolders — the same allowlisted
// SYNO.FileStation.List, asked with filetype=file. Exercised against synomock so
// the request shape is checked against something that answers like DSM.
func TestFileStationListFiles(t *testing.T) {
	c := newTestClient(t)
	sid := login(t, c)
	ctx := context.Background()

	seedTree(t, c, map[string][]string{
		"/movie/Dune (2021)":    {"Dune.2021.mkv", "poster.jpg"},
		"/movie/Arrival (2016)": {"poster.jpg", "Arrival.srt"},
	})

	files, err := c.ListFiles(ctx, sid, "/movie/Dune (2021)")
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("ListFiles = %v, want 2 entries", files)
	}

	// Directories must NOT come back: a season subfolder is not a file, and
	// counting one as content is the bug this whole change exists to fix.
	dirs, err := c.ListFolder(ctx, sid, "/movie")
	if err != nil {
		t.Fatalf("ListFolder: %v", err)
	}
	for _, f := range dirs {
		for _, name := range files {
			if f.Name == name {
				t.Errorf("%q came back from both ListFolder and ListFiles", name)
			}
		}
	}

	// A folder with no files answers empty, not an error — "nothing here" is a
	// valid answer and must be distinguishable from a failed read (FR-010c).
	empty, err := c.ListFiles(ctx, sid, "/movie/4K")
	if err != nil {
		t.Fatalf("ListFiles(empty): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("ListFiles(empty) = %v, want none", empty)
	}

	if _, err := c.ListFiles(ctx, sid, "/movie/Nope"); AsError(err) == nil {
		t.Error("ListFiles on a missing folder should surface a NAS error")
	}
}

// seedTree loads files into the mock through its control endpoint.
func seedTree(t *testing.T, c *HTTPClient, tree map[string][]string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"tree": tree})
	resp, err := http.Post(c.base+"/__mock/library", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("seed returned %d", resp.StatusCode)
	}
}
