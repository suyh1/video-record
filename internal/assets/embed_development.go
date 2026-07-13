//go:build !production

package assets

import (
	"embed"
	"io/fs"
)

//go:embed fallback/index.html
var developmentFiles embed.FS

func distributionFS() fs.FS {
	files, err := fs.Sub(developmentFiles, "fallback")
	if err != nil {
		panic(err)
	}
	return files
}
