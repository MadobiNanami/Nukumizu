package web

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

var staticFS http.FileSystem

// This init runs after embed.go's, which sets StaticFiles (Go initializes a
// package's files in lexical file-name order). StaticFiles is already rooted
// at frontend/dist, so it is served as-is — no further fs.Sub is needed.
func init() {
	staticFS = http.FS(StaticFiles)
}

func ServeStatic(w http.ResponseWriter, r *http.Request) {
	urlPath := r.URL.Path

	if urlPath == "/" || urlPath == "/index.html" || strings.HasPrefix(urlPath, "/assets/") {
		if urlPath == "/" {
			urlPath = "/index.html"
		}

		filePath := strings.TrimPrefix(urlPath, "/")
		f, err := staticFS.Open(filePath)
		if err != nil {
			serveIndexHTML(w)
			return
		}
		defer f.Close()

		stat, err := f.Stat()
		if err != nil {
			serveIndexHTML(w)
			return
		}

		if stat.IsDir() {
			serveIndexHTML(w)
			return
		}

		setContentType(w, path.Ext(filePath))
		http.ServeContent(w, r, filePath, stat.ModTime(), f)
		return
	}

	serveIndexHTML(w)
}

func serveIndexHTML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	data, err := fs.ReadFile(StaticFiles, "index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

func setContentType(w http.ResponseWriter, ext string) {
	switch ext {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	case ".json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".ico":
		w.Header().Set("Content-Type", "image/x-icon")
	case ".woff":
		w.Header().Set("Content-Type", "font/woff")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	case ".ttf":
		w.Header().Set("Content-Type", "font/ttf")
	case ".eot":
		w.Header().Set("Content-Type", "application/vnd.ms-fontobject")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
}
