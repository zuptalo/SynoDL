// Package syno is the Download Station client: the single place that talks to
// the NAS. The APIs implemented here ARE the proxy's allowlist (constitution
// Principle III) — the server exposes typed /v1 endpoints that call this
// interface, never a transparent /webapi passthrough, so nothing outside this
// package can reach the NAS.
package syno

import (
	"context"
	"io"
)

// Client is the surface the HTTP handlers depend on. Tests substitute a fake;
// the real implementation is HTTPClient (http.go).
type Client interface {
	// Login authenticates against DSM and returns the session id. otp is the
	// 2FA code and may be empty.
	Login(ctx context.Context, account, password, otp string) (sid string, err error)
	Logout(ctx context.Context, sid string) error

	ListTasks(ctx context.Context, sid string) ([]Task, error)
	CreateTaskURIs(ctx context.Context, sid string, uris []string, opts CreateOpts) error
	CreateTaskFile(ctx context.Context, sid, filename string, file io.Reader, opts CreateOpts) error
	PauseTasks(ctx context.Context, sid string, ids []string) error
	ResumeTasks(ctx context.Context, sid string, ids []string) error
	DeleteTasks(ctx context.Context, sid string, ids []string) error
	Stats(ctx context.Context, sid string) (Stats, error)

	ListShares(ctx context.Context, sid string) ([]Folder, error)
	ListFolder(ctx context.Context, sid, path string) ([]Folder, error)
	CreateFolder(ctx context.Context, sid, path, name string) (Folder, error)
	// UploadFile streams a file into an existing folder (spec 1022). The only
	// call in this interface that writes file CONTENT to the NAS; everything
	// else reads, or creates an empty folder. The name must already have been
	// validated as a single path segment by the caller.
	UploadFile(ctx context.Context, sid, destFolder, filename string, size int64, overwrite bool, body io.Reader) error
}

// Task is the wire shape served to the PWA (camelCase JSON). It flattens the
// DSM task + its "detail"/"transfer" additionals into one flat record.
type Task struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`   // bt, http, ftp, nzb, emule
	Status        string `json:"status"` // downloading, paused, finished, seeding, error, …
	Size          int64  `json:"size"`
	Downloaded    int64  `json:"downloaded"`
	Uploaded      int64  `json:"uploaded"`
	DownloadSpeed int64  `json:"downloadSpeed"`
	UploadSpeed   int64  `json:"uploadSpeed"`
	Peers         int    `json:"peers"`
	Seeders       int    `json:"seeders"`
	CreatedAt     int64  `json:"createdAt"` // unix seconds
	Destination   string `json:"destination"`
	URI           string `json:"uri,omitempty"`         // source URL/magnet, for copy + re-download
	ErrorDetail   string `json:"errorDetail,omitempty"` // DSM status_extra.error_detail for errored tasks
}

// Stats is the global transfer rate pair shown in the Tasks header.
type Stats struct {
	DownloadSpeed int64 `json:"downloadSpeed"`
	UploadSpeed   int64 `json:"uploadSpeed"`
}

// Folder is one entry of the destination folder picker. Path is the DSM
// FileStation path ("/tv-show/Friends"); the Download Station destination
// parameter wants it without the leading slash.
type Folder struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// CreateOpts carries the optional new-task parameters. Username/Password are
// credentials for the download SOURCE (file-hosting links), forwarded to the
// NAS and never retained; UnzipPassword is the archive extract password.
type CreateOpts struct {
	Destination   string
	Username      string
	Password      string
	UnzipPassword string
}
