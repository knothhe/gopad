//go:build darwin

package windowing

import "github.com/ebitengine/purego/objc"

func raiseOverlay(handle uintptr) {
	// MyGo's ordinary Panel level is NSFloatingWindowLevel (3), below the
	// Dock (20) and menu bar (24). Use the same popup level as MyGo flyouts
	// so this temporary launchpad covers the entire display without
	// changing NSApplication presentation options or creating a Space.
	const popupMenuWindowLevel = 101
	objc.ID(handle).Send(objc.RegisterName("setLevel:"), popupMenuWindowLevel)
}
