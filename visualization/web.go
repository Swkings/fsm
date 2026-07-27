package visualization

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:generate npm --prefix frontend ci
//go:generate npm --prefix frontend run build

// frontendBundle contains the production build of the React application. Keeping
// the bundle in the Go package preserves the single-binary deployment model while
// frontend development remains independent through Vite's API proxy.
//
//go:embed frontend/dist
var frontendBundle embed.FS

var frontendFiles = func() fs.FS {
	files, err := fs.Sub(frontendBundle, "frontend/dist")
	if err != nil {
		panic(err)
	}
	return files
}()

func newFrontendHandler() http.Handler {
	fileServer := http.FileServer(http.FS(frontendFiles))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/api/") {
			http.NotFound(writer, request)
			return
		}
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		assetPath := strings.TrimPrefix(path.Clean("/"+request.URL.Path), "/")
		if assetPath == "." || assetPath == "" {
			assetPath = "index.html"
		}
		if _, err := fs.Stat(frontendFiles, assetPath); err != nil {
			// Extension-less paths belong to the React router. Missing files remain
			// proper 404s instead of receiving HTML as a JavaScript response.
			if path.Ext(assetPath) != "" {
				http.NotFound(writer, request)
				return
			}
			assetPath = "index.html"
		}

		if assetPath == "index.html" {
			content, err := fs.ReadFile(frontendFiles, assetPath)
			if err != nil {
				http.Error(writer, "frontend index is unavailable", http.StatusInternalServerError)
				return
			}
			writer.Header().Set("Cache-Control", "no-cache")
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			if request.Method != http.MethodHead {
				_, _ = writer.Write(content)
			}
			return
		}
		if strings.HasPrefix(assetPath, "assets/") {
			writer.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		clone := request.Clone(request.Context())
		clone.URL.Path = "/" + assetPath
		fileServer.ServeHTTP(writer, clone)
	})
}
