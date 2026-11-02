//go:build darwin

package windowing

import "github.com/ebitengine/purego/objc"

func prepareOverlay(handle uintptr) {
	window := objc.ID(handle)
	// MyGo's Frameless option hides the title bar but keeps NSWindowStyleMaskTitled.
	// AppKit moves titled panels below the menu bar when they are ordered onscreen,
	// keeping their full display height and pushing the bottom offscreen. Make this
	// overlay truly borderless before placement, preserving its nonactivating style.
	const titledWindowStyle = 1 << 0
	style := window.Send(objc.RegisterName("styleMask"))
	if style&titledWindowStyle != 0 {
		window.Send(objc.RegisterName("setStyleMask:"), style&^titledWindowStyle)
	}

	// MyGo's ordinary Panel level is NSFloatingWindowLevel (3), below the
	// Dock (20) and menu bar (24). Use the same popup level as MyGo flyouts
	// so this temporary launchpad covers the entire display without
	// changing NSApplication presentation options or creating a Space.
	const popupMenuWindowLevel = 101
	window.Send(objc.RegisterName("setLevel:"), popupMenuWindowLevel)
}
