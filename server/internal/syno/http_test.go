package syno

import (
	"context"
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
