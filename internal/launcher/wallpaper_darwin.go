//go:build darwin

package launcher

import (
	"errors"
	"math"

	"github.com/ebitengine/purego/objc"
)

// Wallpaper returns a PNG of the main screen's current desktop image. Call it
// on the application's UI thread after AppKit is ready: NSScreen.mainScreen is
// a UI query. It reads only the local file URL supplied by NSWorkspace, requires
// no screen capture permission, and handles HEIC through NSImage. Dynamic/video
// wallpapers without an available still image return an error so the view can
// keep its gradient background.
func Wallpaper() ([]byte, error) {
	imageMu.Lock()
	defer imageMu.Unlock()
	var png []byte
	err := withCocoa(func() {
		screen := message(class("NSScreen"), "mainScreen")
		if screen == 0 {
			return
		}
		workspace := message(class("NSWorkspace"), "sharedWorkspace")
		url := message(workspace, "desktopImageURLForScreen:", screen)
		if url == 0 || message(url, "isFileURL") == 0 {
			return
		}
		image := message(message(class("NSImage"), "alloc"), "initWithContentsOfURL:", url)
		if image == 0 {
			return
		}
		defer message(image, "release")
		if message(image, "isValid") == 0 {
			return
		}
		size := objc.Send[nsSize](image, objc.RegisterName("size"))
		if size.Width <= 0 || size.Height <= 0 || math.IsNaN(size.Width) || math.IsNaN(size.Height) || math.IsInf(size.Width, 0) || math.IsInf(size.Height, 0) {
			return
		}
		// A blurred full-screen backdrop does not need the original 6K image.
		// Bound both dimensions to keep the image and GPU texture inexpensive.
		scale := math.Min(1, 2048/math.Max(size.Width, size.Height))
		width := max(1, int(math.Round(size.Width*scale)))
		height := max(1, int(math.Round(size.Height*scale)))
		png = renderImagePNG(image, width, height)
	})
	if err != nil {
		return nil, err
	}
	if len(png) == 0 {
		return nil, errors.New("the current desktop wallpaper has no readable still image")
	}
	return png, nil
}
