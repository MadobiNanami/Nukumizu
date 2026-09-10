package web

import (
	"io/fs"
	"os"
	"path/filepath"
)

// StaticFiles is the built frontend, rooted at the directory Vite writes to
// (frontend/dist), so paths inside it are relative to that directory, e.g.
// "index.html" or "assets/app.js". The path is relative to the working
// directory, so the server is expected to run from the repository root.
var StaticFiles fs.FS

func init() {
	dir := filepath.Join("frontend", "dist")
	if _, err := os.Stat(dir); err == nil {
		StaticFiles = os.DirFS(dir)
		return
	}

	StaticFiles = os.DirFS(".")
}
