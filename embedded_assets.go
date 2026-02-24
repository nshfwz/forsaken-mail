package mailassets

import (
	"embed"
	"io/fs"
)

//go:embed public
var embeddedAssets embed.FS

func StaticFS() (fs.FS, error) {
	return fs.Sub(embeddedAssets, "public")
}
