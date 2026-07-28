package api

import (
	"container/list"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"synodl/server/internal/source"
)

// Poster/cover proxy + cache. Discover shows many cover images from the
// provider's public CDN; proxying them same-origin (a) sidesteps any CORS/hotlink
// quirks and (b) lets the server serve repeats from memory. It is deliberately
// UNAUTHENTICATED (an <img> tag can't send the session header) but bounded to the
// provider's known image hosts, so it is never an open proxy. The images are
// public covers — nothing sensitive crosses it.

type cachedImage struct {
	body        []byte
	contentType string
}

// imageCache is a tiny byte-bounded LRU so a busy grid re-serves posters from
// memory instead of re-fetching the CDN. Bounded by total bytes, not entries.
type imageCache struct {
	mu       sync.Mutex
	ll       *list.List // front = most recently used
	items    map[string]*list.Element
	bytes    int
	maxBytes int
}

type imageEntry struct {
	key string
	img cachedImage
}

func newImageCache(maxBytes int) *imageCache {
	return &imageCache{ll: list.New(), items: map[string]*list.Element{}, maxBytes: maxBytes}
}

func (c *imageCache) get(key string) (cachedImage, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		return el.Value.(*imageEntry).img, true
	}
	return cachedImage{}, false
}

func (c *imageCache) put(key string, img cachedImage) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.ll.MoveToFront(el)
		return
	}
	// A single oversized image is served but not cached (it would evict the world).
	if len(img.body) > c.maxBytes/2 {
		return
	}
	el := c.ll.PushFront(&imageEntry{key: key, img: img})
	c.items[key] = el
	c.bytes += len(img.body)
	for c.bytes > c.maxBytes {
		back := c.ll.Back()
		if back == nil {
			break
		}
		e := back.Value.(*imageEntry)
		c.ll.Remove(back)
		delete(c.items, e.key)
		c.bytes -= len(e.img.body)
	}
}

// posterCache holds ~64 MB of covers (covers are ~50–400 KB, so hundreds).
var posterCache = newImageCache(64 << 20)

// posterClient fetches CDN images. No session/browser-emulation needed — covers
// are public; a Referer keeps us clear of any hotlink protection.
var posterClient = &http.Client{Timeout: 20 * time.Second}

// handleSourceImage proxies a provider cover image (unauthenticated; host-bounded).
func handleSourceImage(d Deps) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.URL.Query().Get("u")
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || !source.ImageHostAllowed(u.Hostname()) {
			http.Error(w, "bad image", http.StatusBadRequest)
			return
		}
		// Long cache: covers are content-addressed by URL and effectively immutable.
		w.Header().Set("Cache-Control", "public, max-age=604800, immutable")

		if img, ok := posterCache.get(raw); ok {
			w.Header().Set("Content-Type", img.contentType)
			w.Header().Set("X-Cache", "HIT")
			_, _ = w.Write(img.body)
			return
		}

		req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, raw, nil)
		req.Header.Set("Referer", "https://"+u.Hostname()+"/")
		req.Header.Set("User-Agent", "Mozilla/5.0 SynoDL")
		resp, err := posterClient.Do(req)
		if err != nil {
			http.Error(w, "image unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			http.Error(w, "image unavailable", http.StatusBadGateway)
			return
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 12<<20)) // cap a single cover at 12 MB
		if err != nil {
			http.Error(w, "image read error", http.StatusBadGateway)
			return
		}
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "image/jpeg"
		}
		posterCache.put(raw, cachedImage{body: body, contentType: ct})
		w.Header().Set("Content-Type", ct)
		w.Header().Set("X-Cache", "MISS")
		_, _ = w.Write(body)
	})
}
