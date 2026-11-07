// Package icon holds the application's icon as PNG images of several sizes,
// drawn by tools/genicon, for windows that set their icon at run time.
package icon

import "embed"

// PNG holds png/icon-<size>.png.
//
//go:embed png/*.png
var PNG embed.FS
