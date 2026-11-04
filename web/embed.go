package web

import (
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

//go:embed dist/*
var DistFS embed.FS

type spaFileSystem struct {
	root http.FileSystem
}

func (s *spaFileSystem) Open(name string) (http.File, error) {
	cleanName := filepath.Clean(name)
	f, err := s.root.Open(cleanName)
	if err != nil {
		if os.IsNotExist(err) || strings.HasSuffix(err.Error(), "file does not exist") {
			return s.root.Open("index.html")
		}
		return nil, err
	}
	return f, nil
}

func AssetHandler() http.Handler {
	sub, err := fs.Sub(DistFS, "dist")
	if err != nil {
		panic(err)
	}
	return http.FileServer(&spaFileSystem{root: http.FS(sub)})
}
