package utils

import (
	"embed"
	"io/fs"
	"net/http"
	"os"

	"github.com/tejzpr/monito/log"
)

// GetFileSystem If the `useOS` flag is set, use the live filesystem, otherwise use the embedded filesystem
func GetFileSystem(useOS bool, embedFs embed.FS) http.FileSystem {
	if useOS {
		log.Info("using live mode")
		return http.FS(os.DirFS("public/static"))
	}

	fsys, err := fs.Sub(embedFs, "public/static")
	if err != nil {
		panic(err)
	}

	return http.FS(fsys)
}
