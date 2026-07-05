package app

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Run `cd web && npm run build` before `go test ./...` or `go build` so
// go:embed can include the current Vite output.
//
//go:embed web/dist web/dist/*
var webDist embed.FS

func newStaticHandler() http.Handler {
	distFS := mustSubFS(webDist, "web/dist")
	fileServer := http.FileServer(http.FS(distFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "." {
			cleanPath = ""
		}

		if cleanPath == "api" || strings.HasPrefix(cleanPath, "api/") {
			http.NotFound(w, r)
			return
		}

		if cleanPath != "" && embeddedFileExists(distFS, cleanPath) {
			fileServer.ServeHTTP(w, r)
			return
		}

		serveEmbeddedIndex(w, r, distFS)
	})
}

func mustSubFS(root fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(root, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func embeddedFileExists(root fs.FS, name string) bool {
	info, err := fs.Stat(root, name)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func serveEmbeddedIndex(w http.ResponseWriter, r *http.Request, root fs.FS) {
	content, err := fs.ReadFile(root, "index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(content)
}
