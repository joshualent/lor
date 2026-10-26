package server

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

// spaHandler serves the embedded SvelteKit build (plan section 11):
//   - /_app/immutable/* → long-lived immutable cache
//   - other files that exist in the bundle → served as is
//   - anything else → index.html with no-cache (deep links like /r/42)
//
// API paths never reach this handler.
type spaHandler struct {
	files fs.FS
	index []byte
}

func newSPA(files fs.FS) (*spaHandler, error) {
	index, err := fs.ReadFile(files, "index.html")
	if err != nil {
		return nil, errors.New("web UI bundle has no index.html (run `make web`)")
	}
	return &spaHandler{files: files, index: index}, nil
}

func (s *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	p := path.Clean("/" + r.URL.Path)
	if strings.HasPrefix(p, "/api/") || p == "/api" {
		jsonNotFound(w, r)
		return
	}
	name := strings.TrimPrefix(p, "/")
	if name != "" && name != "index.html" {
		if f, err := s.files.Open(name); err == nil {
			defer f.Close()
			if st, err := f.Stat(); err == nil && !st.IsDir() {
				if strings.HasPrefix(p, "/_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				if rs, ok := f.(io.ReadSeeker); ok {
					http.ServeContent(w, r, name, time.Time{}, rs)
					return
				}
				data, err := io.ReadAll(f)
				if err == nil {
					http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
					return
				}
			}
		}
	}
	// SPA fallback: index.html for every other path.
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(s.index))
}
