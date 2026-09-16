package synomock

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// A fake IMDb, so spec 0014 is exercisable with no network (FR-035).
//
// It serves the two things the real one does for this feature: a person page
// whose <head> names their photograph, and the photograph. It can also be told
// to refuse, which is the state that proves the promise the whole spec is
// arranged around — that losing the fallback entirely costs faces and nothing
// else.
//
//	GET  /mockimdb/name/{nm}/   a person page with (or without) an og:image
//	GET  /mockimg/{nm}.jpg      the photograph
//	POST /__mock/imdb/down      start refusing every lookup
//	POST /__mock/imdb/up        answer again
//	GET  /__mock/imdb/hits      how many lookups have been made
//
// The hit counter is what lets a test assert the thing the cache exists for: a
// person seen in a second title costs no second lookup.

// nmWithoutPhoto is a person the fake IMDb knows of but has no picture of — so
// "IMDb has no photograph of them" is a tested state rather than an assumed one.
const nmWithoutPhoto = "nm0000999"

type imdbState struct {
	mu   sync.Mutex
	down bool
	hits atomic.Int64
}

func (s *Server) registerIMDbMock(mux *http.ServeMux) {
	mux.HandleFunc("GET /mockimdb/name/{nm}/", s.handleMockIMDbName)
	mux.HandleFunc("GET /mockimg/{file}", s.handleMockIMDbImage)
	mux.HandleFunc("POST /__mock/imdb/{state}", s.handleMockIMDbState)
	mux.HandleFunc("GET /__mock/imdb/hits", s.handleMockIMDbHits)
}

func (s *Server) handleMockIMDbName(w http.ResponseWriter, r *http.Request) {
	s.imdb.mu.Lock()
	down := s.imdb.down
	s.imdb.mu.Unlock()
	if down {
		// The real thing refuses with a status, not a connection error, when it
		// decides it does not like you.
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	s.imdb.hits.Add(1)

	nm := strings.TrimSuffix(r.PathValue("nm"), "/")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if nm == nmWithoutPhoto {
		fmt.Fprintf(w, `<html><head><title>%s - IMDb</title></head><body>no picture of them</body></html>`, nm)
		return
	}
	// og:image sits in the head, and the body is enormous — which is why the
	// reader stops at </head>. The fake is shaped the same way so that bound is
	// exercised rather than asserted.
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	base := scheme + "://" + r.Host
	fmt.Fprintf(w, `<html><head><title>%s - IMDb</title>`+
		`<meta property="og:image" content="%s/mockimg/%s.jpg">`+
		`</head><body>%s</body></html>`,
		nm, base, nm, strings.Repeat("padding ", 4096))
}

// handleMockIMDbImage serves a one-pixel JPEG. The bytes do not matter; that it
// is an image with an image content type, cached and re-served by the proxy in
// front of it, does.
func (s *Server) handleMockIMDbImage(w http.ResponseWriter, r *http.Request) {
	s.imdb.mu.Lock()
	down := s.imdb.down
	s.imdb.mu.Unlock()
	if down {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	_, _ = w.Write(onePixelJPEG)
}

func (s *Server) handleMockIMDbState(w http.ResponseWriter, r *http.Request) {
	s.imdb.mu.Lock()
	switch r.PathValue("state") {
	case "down":
		s.imdb.down = true
	case "up":
		s.imdb.down = false
	case "reset":
		s.imdb.down = false
		s.imdb.hits.Store(0)
	}
	s.imdb.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleMockIMDbHits(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, `{"hits":%d}`, s.imdb.hits.Load())
}

// onePixelJPEG is the smallest valid JPEG: a 1×1 white pixel.
var onePixelJPEG = []byte{
	0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00,
	0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43, 0x00, 0x08, 0x06,
	0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09, 0x09, 0x08, 0x0A, 0x0C,
	0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12, 0x13, 0x0F, 0x14, 0x1D, 0x1A,
	0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20, 0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C,
	0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29, 0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F,
	0x27, 0x39, 0x3D, 0x38, 0x32, 0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00,
	0x0B, 0x08, 0x00, 0x01, 0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00,
	0x1F, 0x00, 0x00, 0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07,
	0x08, 0x09, 0x0A, 0x0B, 0xFF, 0xC4, 0x00, 0x14, 0x10, 0x01, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF,
	0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00, 0x37, 0xFF, 0xD9,
}
