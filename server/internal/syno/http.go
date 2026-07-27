package syno

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DSM API names — this list IS the proxy allowlist. Adding an entry is a
// spec-level decision (constitution, Domain Constraints).
const (
	apiInfo   = "SYNO.API.Info"
	apiAuth   = "SYNO.API.Auth"
	apiTask   = "SYNO.DownloadStation.Task"
	apiStat     = "SYNO.DownloadStation.Statistic"
	apiFSList   = "SYNO.FileStation.List"
	apiFSCreate = "SYNO.FileStation.CreateFolder"
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
}

// HTTPClient is the real DSM client. It is safe for concurrent use; the only
// mutable state is the discovered API table (path + max version per API),
// fetched lazily once and kept in memory — never on disk (Principle III).
type HTTPClient struct {
	base string // e.g. https://nas.local:5001
	hc   *http.Client

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
			// small; a generous timeout still protects against a hung NAS.
			Timeout: 60 * time.Second,
		},
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
			"query":   {strings.Join([]string{apiAuth, apiTask, apiStat, apiFSList, apiFSCreate}, ",")},
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
