package syno

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DSM API names — this list IS the proxy allowlist. Adding an entry is a
// spec-level decision (constitution, Domain Constraints).
const (
	apiInfo     = "SYNO.API.Info"
	apiAuth     = "SYNO.API.Auth"
	apiTask     = "SYNO.DownloadStation.Task"
	apiStat     = "SYNO.DownloadStation.Statistic"
	apiFSList   = "SYNO.FileStation.List"
	apiFSCreate = "SYNO.FileStation.CreateFolder"
	// The one API that writes file CONTENT to the NAS (spec 1022). Everything
	// else here reads, or creates an empty folder.
	apiFSUpload = "SYNO.FileStation.Upload"
)

// maxSupported caps the API version we negotiate: we speak min(our max, the
// NAS's max). This is how one binary covers DSM 6 and DSM 7 — the NAS tells us
// its paths and versions via SYNO.API.Info and we meet it where it is.
var maxSupported = map[string]int{
	apiAuth:     6,
	apiTask:     3,
	apiStat:     1,
	apiFSList:   2,
	apiFSCreate: 2,
	apiFSUpload: 2,
}

// discoverableAPIs is every API this client may call, sorted so the discovery
// request is byte-identical run to run. Sourced from maxSupported so the set the
// client negotiates versions for and the set it asks the NAS about can never
// drift apart.
func discoverableAPIs() []string {
	out := make([]string, 0, len(maxSupported))
	for api := range maxSupported {
		out = append(out, api)
	}
	sort.Strings(out)
	return out
}

// HTTPClient is the real DSM client. It is safe for concurrent use; the only
// mutable state is the discovered API table (path + max version per API),
// fetched lazily once and kept in memory — never on disk (Principle III).
type HTTPClient struct {
	base string // e.g. https://nas.local:5001
	hc   *http.Client
	// A second client for uploads. hc carries a 60s timeout, which is right for
	// the small calls everything else makes and would sever a multi-gigabyte
	// upload mid-transfer. This one has no deadline of its own; the request
	// context governs it, so a cancelled or dropped upload still stops promptly.
	uploadHC *http.Client

	mu   sync.Mutex
	apis map[string]endpoint
}

type endpoint struct {
	Path       string `json:"path"`
	MaxVersion int    `json:"maxVersion"`
}

// NewHTTPClient builds a client for the given DSM base URL. tlsInsecure
// disables certificate verification for this outbound connection only —
// an explicit operator opt-in for self-signed NAS certs (SYNO_TLS_INSECURE).
func NewHTTPClient(baseURL string, tlsInsecure bool) *HTTPClient {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if tlsInsecure {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &HTTPClient{
		base: strings.TrimRight(baseURL, "/"),
		hc: &http.Client{
			Transport: transport,
			// Downloads run on the NAS, not through us, so every proxied call is
			// small; a generous timeout still protects against a hung NAS. The
			// one exception is an upload, which uses uploadHC below.
			Timeout: 60 * time.Second,
		},
		uploadHC: &http.Client{Transport: transport},
	}
}

// endpointFor resolves the discovered path+version for an API, running the
// SYNO.API.Info query on first use. On any discovery failure the cache stays
// empty so the next request retries.
func (c *HTTPClient) endpointFor(ctx context.Context, api string) (endpoint, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.apis == nil {
		q := url.Values{
			"api":     {apiInfo},
			"version": {"1"},
			"method":  {"query"},
			// Derived from maxSupported rather than written out again. The two
			// lists MUST agree: an API the client can call but never asks about
			// is absent from c.apis, so endpointFor fails before any request is
			// made. That is exactly what happened to SYNO.FileStation.Upload —
			// added to maxSupported by spec 1022, never added here, so every
			// upload on a real NAS failed with a code-less error while the mock
			// (which ignored this filter) passed every test. One list, so a new
			// API cannot be half-registered.
			"query": {strings.Join(discoverableAPIs(), ",")},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			c.base+"/webapi/query.cgi?"+q.Encode(), nil)
		if err != nil {
			return endpoint{}, err
		}
		resp, err := c.hc.Do(req)
		if err != nil {
			return endpoint{}, &Error{Kind: KindUnreachable, API: apiInfo}
		}
		defer resp.Body.Close()
		var env struct {
			Success bool                `json:"success"`
			Data    map[string]endpoint `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&env); err != nil || !env.Success {
			return endpoint{}, &Error{Kind: KindUnreachable, API: apiInfo}
		}
		c.apis = env.Data
	}
	ep, ok := c.apis[api]
	if !ok || ep.Path == "" {
		// The NAS doesn't offer this API (e.g. Download Station not installed).
		return endpoint{}, &Error{Kind: KindNAS, API: api}
	}
	return ep, nil
}

// call performs one allowlisted DSM request as a POST form (keeping the
// password/sid out of URLs and therefore out of any access log) and decodes
// the standard {success,data,error} envelope into out.
func (c *HTTPClient) call(ctx context.Context, api, method, sid string, params url.Values, out any) error {
	ep, err := c.endpointFor(ctx, api)
	if err != nil {
		return err
	}
	form := url.Values{
		"api":     {api},
		"version": {fmt.Sprint(min(maxSupported[api], ep.MaxVersion))},
		"method":  {method},
	}
	if sid != "" {
		form.Set("_sid", sid)
	}
	for k, vs := range params {
		form[k] = vs
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/webapi/"+ep.Path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.hc.Do(req)
	if err != nil {
		return &Error{Kind: KindUnreachable, API: api}
	}
	defer resp.Body.Close()
	return decodeEnvelope(resp.Body, api, out)
}

func decodeEnvelope(r io.Reader, api string, out any) error {
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
		Err     struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(r).Decode(&env); err != nil {
		return &Error{Kind: KindUnreachable, API: api}
	}
	if !env.Success {
		return &Error{Kind: classify(api, env.Err.Code), Code: env.Err.Code, API: api}
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return &Error{Kind: KindNAS, API: api}
		}
	}
	return nil
}

func (c *HTTPClient) Login(ctx context.Context, account, password, otp string) (string, error) {
	params := url.Values{
		"account": {account},
		"passwd":  {password},
		"session": {"DownloadStation"},
		"format":  {"sid"},
	}
	if otp != "" {
		params.Set("otp_code", otp)
	}
	var data struct {
		SID string `json:"sid"`
	}
	if err := c.call(ctx, apiAuth, "login", "", params, &data); err != nil {
		return "", err
	}
	return data.SID, nil
}

func (c *HTTPClient) Logout(ctx context.Context, sid string) error {
	return c.call(ctx, apiAuth, "logout", sid, url.Values{"session": {"DownloadStation"}}, nil)
}

// dsmTask is the raw DSM task shape ("additional" split into detail/transfer);
// flattened into the wire Task before it leaves this package.
type dsmTask struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Type   string `json:"type"`
	Status string `json:"status"`
	Size   int64  `json:"size"`
	// DSM reports a failed task's cause here (sibling of status/additional).
	StatusExtra struct {
		ErrorDetail string `json:"error_detail"`
	} `json:"status_extra"`
	Additional struct {
		Detail struct {
			CreateTime       int64  `json:"create_time"`
			Destination      string `json:"destination"`
			URI              string `json:"uri"` // source URL/magnet for copy + re-download
			ConnectedPeers   int    `json:"connected_peers"`
			ConnectedSeeders int    `json:"connected_seeders"`
		} `json:"detail"`
		Transfer struct {
			SizeDownloaded int64 `json:"size_downloaded"`
			SizeUploaded   int64 `json:"size_uploaded"`
			SpeedDownload  int64 `json:"speed_download"`
			SpeedUpload    int64 `json:"speed_upload"`
		} `json:"transfer"`
	} `json:"additional"`
}

func (c *HTTPClient) ListTasks(ctx context.Context, sid string) ([]Task, error) {
	var data struct {
		Tasks []dsmTask `json:"tasks"`
	}
	params := url.Values{
		"additional": {"detail,transfer"},
		"limit":      {"-1"},
	}
	if err := c.call(ctx, apiTask, "list", sid, params, &data); err != nil {
		return nil, err
	}
	tasks := make([]Task, 0, len(data.Tasks))
	for _, t := range data.Tasks {
		tasks = append(tasks, Task{
			ID:            t.ID,
			Name:          t.Title,
			Type:          t.Type,
			Status:        t.Status,
			Size:          t.Size,
			Downloaded:    t.Additional.Transfer.SizeDownloaded,
			Uploaded:      t.Additional.Transfer.SizeUploaded,
			DownloadSpeed: t.Additional.Transfer.SpeedDownload,
			UploadSpeed:   t.Additional.Transfer.SpeedUpload,
			Peers:         t.Additional.Detail.ConnectedPeers,
			Seeders:       t.Additional.Detail.ConnectedSeeders,
			CreatedAt:     t.Additional.Detail.CreateTime,
			Destination:   t.Additional.Detail.Destination,
			URI:           t.Additional.Detail.URI,
			ErrorDetail:   t.StatusExtra.ErrorDetail,
		})
	}
	return tasks, nil
}

func createParams(opts CreateOpts) url.Values {
	params := url.Values{}
	if opts.Destination != "" {
		params.Set("destination", opts.Destination)
	}
	if opts.Username != "" {
		params.Set("username", opts.Username)
	}
	if opts.Password != "" {
		params.Set("password", opts.Password)
	}
	if opts.UnzipPassword != "" {
		params.Set("unzip_password", opts.UnzipPassword)
	}
	return params
}

func (c *HTTPClient) CreateTaskURIs(ctx context.Context, sid string, uris []string, opts CreateOpts) error {
	params := createParams(opts)
	params.Set("uri", strings.Join(uris, ","))
	return c.call(ctx, apiTask, "create", sid, params, nil)
}

// CreateTaskFile uploads a .torrent (or NZB) via multipart. Unlike call(), the
// body is multipart form-data because DSM only accepts task files that way; all
// scalar params ride as form fields alongside the file part.
func (c *HTTPClient) CreateTaskFile(ctx context.Context, sid, filename string, file io.Reader, opts CreateOpts) error {
	ep, err := c.endpointFor(ctx, apiTask)
	if err != nil {
		return err
	}
	buf := &bytes.Buffer{}
	mw := multipart.NewWriter(buf)
	fields := createParams(opts)
	fields.Set("api", apiTask)
	fields.Set("version", fmt.Sprint(min(maxSupported[apiTask], ep.MaxVersion)))
	fields.Set("method", "create")
	fields.Set("_sid", sid)
	for k, vs := range fields {
		for _, v := range vs {
			if err := mw.WriteField(k, v); err != nil {
				return err
			}
		}
	}
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, file); err != nil {
		return err
	}
	if err := mw.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/webapi/"+ep.Path, buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := c.hc.Do(req)
	if err != nil {
		return &Error{Kind: KindUnreachable, API: apiTask}
	}
	defer resp.Body.Close()
	return decodeEnvelope(resp.Body, apiTask, nil)
}

func (c *HTTPClient) PauseTasks(ctx context.Context, sid string, ids []string) error {
	return c.call(ctx, apiTask, "pause", sid, url.Values{"id": {strings.Join(ids, ",")}}, nil)
}

func (c *HTTPClient) ResumeTasks(ctx context.Context, sid string, ids []string) error {
	return c.call(ctx, apiTask, "resume", sid, url.Values{"id": {strings.Join(ids, ",")}}, nil)
}

func (c *HTTPClient) DeleteTasks(ctx context.Context, sid string, ids []string) error {
	params := url.Values{
		"id": {strings.Join(ids, ",")},
		// Keep completed files on the NAS; deleting a task is not deleting data.
		"force_complete": {"false"},
	}
	return c.call(ctx, apiTask, "delete", sid, params, nil)
}

func (c *HTTPClient) Stats(ctx context.Context, sid string) (Stats, error) {
	var data struct {
		SpeedDownload int64 `json:"speed_download"`
		SpeedUpload   int64 `json:"speed_upload"`
	}
	if err := c.call(ctx, apiStat, "getinfo", sid, nil, &data); err != nil {
		return Stats{}, err
	}
	return Stats{DownloadSpeed: data.SpeedDownload, UploadSpeed: data.SpeedUpload}, nil
}

type dsmFile struct {
	IsDir bool   `json:"isdir"`
	Name  string `json:"name"`
	Path  string `json:"path"`
}

// ListFiles returns the FILE names in a folder — the same allowlisted
// SYNO.FileStation.List that ListFolder uses, asked with filetype=file.
//
// Ownership turns on what a folder CONTAINS, not on its name (FR-001a), and
// ListFolder deliberately asks for directories only, so this read did not exist.
// It is also why spec 0008's security checklist re-runs: the server now sees FILE
// names where it previously saw only folder names. Those names are used to decide
// presence and are never logged.
func (c *HTTPClient) ListFiles(ctx context.Context, sid, path string) ([]string, error) {
	var data struct {
		Files []dsmFile `json:"files"`
	}
	params := url.Values{
		"folder_path": {path},
		"filetype":    {"file"},
	}
	if err := c.call(ctx, apiFSList, "list", sid, params, &data); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(data.Files))
	for _, f := range data.Files {
		// Defensive: a NAS that ignores filetype must not have its directories
		// counted as content, which is the precise failure being corrected.
		if f.IsDir {
			continue
		}
		names = append(names, f.Name)
	}
	return names, nil
}

func (c *HTTPClient) ListShares(ctx context.Context, sid string) ([]Folder, error) {
	var data struct {
		Shares []dsmFile `json:"shares"`
	}
	if err := c.call(ctx, apiFSList, "list_share", sid, nil, &data); err != nil {
		return nil, err
	}
	return toFolders(data.Shares), nil
}

func (c *HTTPClient) ListFolder(ctx context.Context, sid, path string) ([]Folder, error) {
	var data struct {
		Files []dsmFile `json:"files"`
	}
	params := url.Values{
		"folder_path": {path},
		"filetype":    {"dir"},
	}
	if err := c.call(ctx, apiFSList, "list", sid, params, &data); err != nil {
		return nil, err
	}
	return toFolders(data.Files), nil
}

// CreateFolder creates a single subfolder `name` under the absolute parent
// `path` and returns it. force_parent is left off (defaults false) so a missing
// parent is an error rather than silently created.
func (c *HTTPClient) CreateFolder(ctx context.Context, sid, path, name string) (Folder, error) {
	var data struct {
		Folders []dsmFile `json:"folders"`
	}
	params := url.Values{
		"folder_path": {path},
		"name":        {name},
	}
	if err := c.call(ctx, apiFSCreate, "create", sid, params, &data); err != nil {
		return Folder{}, err
	}
	if len(data.Folders) > 0 {
		return Folder{Name: data.Folders[0].Name, Path: data.Folders[0].Path}, nil
	}
	// Some DSM versions return an empty body on success; synthesize the folder.
	return Folder{Name: name, Path: strings.TrimRight(path, "/") + "/" + name}, nil
}

func toFolders(files []dsmFile) []Folder {
	out := make([]Folder, 0, len(files))
	for _, f := range files {
		if !f.IsDir {
			continue
		}
		out = append(out, Folder{Name: f.Name, Path: f.Path})
	}
	return out
}

// UploadFile streams a file into a folder on the NAS (spec 1022).
//
// The body is piped straight from the caller's reader into the multipart
// request, so a two-gigabyte file never exists in this process's memory — the
// difference from CreateTaskFile above, which buffers because a .torrent is
// kilobytes. DSM requires the file part LAST, after the fields.
//
// `overwrite` is deliberately NOT sent. DSM's default is to fail when the file
// already exists, which is what we want: a collision must be reported, never
// resolved by silently replacing somebody's file or silently dropping the
// upload.
//
// The file name is used exactly as given. Callers MUST have validated it as a
// single path segment first (library.ValidUploadName) — it is client-supplied
// text that becomes part of a path on the NAS.
func (c *HTTPClient) UploadFile(
	ctx context.Context, sid, destFolder, filename string, size int64, overwrite bool,
	body io.Reader,
) error {
	ep, err := c.endpointFor(ctx, apiFSUpload)
	if err != nil {
		return err
	}
	// Dispatch and authentication ride in the QUERY STRING, not the multipart
	// body. DSM's entry.cgi resolves the API and the session BEFORE it parses a
	// multipart body, so a _sid sent as a form field is never seen and the call
	// is rejected with 119 ("sid not found") — even though the very same sid
	// works for every non-multipart call. Only the operands belong in the body.
	q := url.Values{
		"api":     {apiFSUpload},
		"version": {fmt.Sprint(min(maxSupported[apiFSUpload], ep.MaxVersion))},
		"method":  {"upload"},
		"_sid":    {sid},
	}
	fields := url.Values{
		"path":           {destFolder},
		"create_parents": {"false"},
		// Off unless the caller explicitly asked. DSM then refuses a name that is
		// already taken (414) rather than replacing it, which turns a collision
		// into a question for the user instead of a silently destroyed file. The
		// one case where replacing is right is recovering a PARTIAL file left by
		// an interrupted upload — and that is the user's call, not a default.
		"overwrite": {strconv.FormatBool(overwrite)},
	}

	// DSM's entry.cgi refuses an upload sent with a chunked request body — it
	// requires a real Content-Length. Streaming the multipart through an io.Pipe
	// leaves the length unknown, so Go falls back to Transfer-Encoding: chunked
	// and the NAS rejects the call before reading a single byte of the file.
	// That is invisible to the mock, whose Go HTTP server accepts chunked
	// happily, so the tests passed while every real upload failed instantly.
	//
	// Compose the envelope AROUND the file instead of through it: everything
	// before the file bytes and everything after are small and exactly known, so
	// the total length can be declared up front while the file itself is still
	// streamed and never buffered (uploads run to gigabytes; the pod has 192Mi).
	var pre bytes.Buffer
	mw := multipart.NewWriter(&pre)
	for k, vs := range fields {
		for _, v := range vs {
			if err := mw.WriteField(k, v); err != nil {
				return err
			}
		}
	}
	// Writes only the part's headers into pre; the content follows from `body`.
	if _, err := mw.CreateFormFile("file", filename); err != nil {
		return err
	}
	// Deliberately NOT mw.Close(): that would append the terminating boundary
	// here, before the file. It is written after the file bytes instead, in
	// exactly the form Close would have produced.
	post := []byte("\r\n--" + mw.Boundary() + "--\r\n")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/webapi/"+ep.Path+"?"+q.Encode(),
		io.MultiReader(&pre, io.LimitReader(body, size), bytes.NewReader(post)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	// The whole point: an exact length, so the request is not chunked.
	req.ContentLength = int64(pre.Len()) + size + int64(len(post))

	resp, err := c.uploadHC.Do(req)
	if err != nil {
		slog.Warn("filestation upload transport error", "err", err.Error())
		return &Error{Kind: KindNAS, API: apiFSUpload}
	}
	defer resp.Body.Close()

	// Upload is the one call seen to answer success:false with NO error code,
	// which leaves nothing to act on. A DSM envelope is a few dozen bytes, so
	// read it and report the HTTP status alongside — the status distinguishes a
	// FileStation refusal from a rejection by the web server in front of it
	// (413/411), which look identical once the body has been discarded.
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<10))
	if e := decodeEnvelope(bytes.NewReader(raw), apiFSUpload, nil); e != nil {
		slog.Warn("filestation upload rejected",
			"status", resp.StatusCode,
			"declared_length", req.ContentLength,
			"body", strings.TrimSpace(string(raw)))
		return e
	}
	return nil
}
