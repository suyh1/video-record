//go:build production

package assets

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var productionFiles embed.FS

func distributionFS() fs.FS {
	files, err := fs.Sub(productionFiles, "dist")
	if err != nil {
		panic(err)
	}
	return files
}
