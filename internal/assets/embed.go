package assets

import (
	"bytes"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
)

func NewHandler(api http.Handler) http.Handler {
	return newHandler(api, distributionFS())
}

func newHandler(api http.Handler, files fs.FS) http.Handler {
	if api == nil {
		api = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delegatesToAPI(r) {
			api.ServeHTTP(w, r)
			return
		}
		requested := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if requested == "" || requested == "." {
			requested = "index.html"
		}
		contents, err := fs.ReadFile(files, requested)
		if err != nil {
			if strings.HasPrefix(requested, "assets/") {
				http.NotFound(w, r)
				return
			}
			requested = "index.html"
			contents, err = fs.ReadFile(files, requested)
			if err != nil {
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if contentType := mime.TypeByExtension(path.Ext(requested)); contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if requested == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		http.ServeContent(w, r, requested, time.Time{}, bytes.NewReader(contents))
	})
}

func delegatesToAPI(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return true
	}
	return r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/") ||
		r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/healthz/") ||
		r.URL.Path == "/readyz" || strings.HasPrefix(r.URL.Path, "/readyz/")
}
