// Package windowing keeps the launcher's temporary screen overlay separate
// from the ordinary settings window.
package windowing

import (
	"github.com/egoist/mygo"
	"github.com/knothhe/gopad/internal/i18n"
)

func LauncherOptions(windowed bool) mygo.WindowOptions {
	options := mygo.WindowOptions{Title: "gopad", Width: 1280, Height: 860, MinWidth: 700, MinHeight: 520, Hidden: true, DisableFullScreen: true, BackgroundColor: "#202c43"}
	if !windowed {
		// A nonactivating panel joins the user's current Space. It never
		// enters macOS full screen or creates another Space.
		options.Panel = true
		options.Frameless = true
		options.AlwaysOnTop = true
		options.DisableMove = true
		options.DisableResize = true
		options.DisableMinimize = true
		options.DisableMaximize = true
		options.DisableShadow = true
	}
	return options
}

func ManagerOptions() mygo.WindowOptions {
	return mygo.WindowOptions{
		Title: i18n.T("en", "app.managerTitle", "name", "gopad"), URL: "/?manage=1", Width: 1080, Height: 760,
		MinWidth: 760, MinHeight: 560, Hidden: true, DisableFullScreen: true,
		BackgroundColor: "#161a20",
	}
}

// PlaceOverlay must be called on the UI thread. Bounds include the menu bar
// and Dock; unlike the work area, they describe the whole selected display.
func PlaceOverlay(window *mygo.Window, bounds mygo.Rectangle) {
	prepareOverlay(window.NativeHandle())
	window.SetBounds(bounds)
}
