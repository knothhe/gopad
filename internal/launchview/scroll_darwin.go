//go:build darwin

package launchview

import (
	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Use a pointer return type for dlsym so no integer-to-Go-pointer conversion
// is needed when reading AppKit's existing NSApp singleton.
var scrollSymbol = func() func(uintptr, string) *objc.ID {
	var lookup func(uintptr, string) *objc.ID
	purego.RegisterLibFunc(&lookup, purego.RTLD_DEFAULT, "dlsym")
	return lookup
}()

// MyGo delivers scroll input synchronously from scrollWheel:. Read AppKit's
// current event here because ui.InputEvent does not expose NSEvent.phase.
func nativeScrollPhase() scrollPhase {
	// Do not initialize AppKit when running headless tests.
	address := scrollSymbol(purego.RTLD_DEFAULT, "NSApp")
	if address == nil || *address == 0 {
		return scrollUnphased
	}
	event := address.Send(objc.RegisterName("currentEvent"))
	if event == 0 || event.Send(objc.RegisterName("type")) != 22 {
		return scrollUnphased
	}
	phase := event.Send(objc.RegisterName("phase"))
	switch {
	case phase&16 != 0:
		return scrollCancelled
	case phase&8 != 0:
		return scrollEnded
	case phase&1 != 0:
		return scrollBegan
	case phase != 0:
		return scrollChanged
	default:
		return scrollUnphased
	}
}
