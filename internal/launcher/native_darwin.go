//go:build darwin

package launcher

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var cocoa struct {
	sync.Once
	err        error
	drawImage  func(objc.ID, objc.SEL, nsRect, nsRect, uint, float64)
	initBitmap func(objc.ID, objc.SEL, uintptr, int, int, int, int, bool, bool, objc.ID, int, int) objc.ID
}

type nsPoint struct{ X, Y float64 }
type nsSize struct{ Width, Height float64 }
type nsRect struct {
	Origin nsPoint
	Size   nsSize
}

func initCocoa() error {
	cocoa.Do(func() {
		for _, library := range []string{"/System/Library/Frameworks/Foundation.framework/Foundation", "/System/Library/Frameworks/AppKit.framework/AppKit"} {
			if _, err := purego.Dlopen(library, purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
				cocoa.err = err
				return
			}
		}
		library, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			cocoa.err = err
			return
		}
		address, err := purego.Dlsym(library, "objc_msgSend")
		if err != nil {
			cocoa.err = err
			return
		}
		purego.RegisterFunc(&cocoa.drawImage, address)
		purego.RegisterFunc(&cocoa.initBitmap, address)
	})
	return cocoa.err
}

func message(id objc.ID, name string, args ...any) objc.ID {
	return id.Send(objc.RegisterName(name), args...)
}
func class(name string) objc.ID { return objc.ID(objc.GetClass(name)) }
func nsString(s string) objc.ID { return message(class("NSString"), "stringWithUTF8String:", s) }
func goString(s objc.ID) string {
	if s == 0 {
		return ""
	}
	// Some third-party bundles accidentally store version numbers as numbers.
	// Sending UTF8String to those objects would raise an Objective-C exception.
	if message(s, "respondsToSelector:", objc.RegisterName("UTF8String")) == 0 {
		return ""
	}
	return objc.Send[string](s, objc.RegisterName("UTF8String"))
}

// AppKit's image operations and autoreleased objects must stay on one OS thread.
// NSWorkspace iconForFile and offscreen NSGraphicsContext do not need NSApp or
// its event loop, which also makes the real application list usable in preview.
func withCocoa(fn func()) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if err := initCocoa(); err != nil {
		return err
	}
	pool := message(message(class("NSAutoreleasePool"), "alloc"), "init")
	defer message(pool, "drain")
	fn()
	return nil
}

func describeApplications(paths []string) ([]App, error) {
	apps := make([]App, 0, len(paths))
	err := withCocoa(func() {
		manager := message(class("NSFileManager"), "defaultManager")
		for _, path := range paths {
			bundle := message(class("NSBundle"), "bundleWithPath:", nsString(path))
			if bundle == 0 {
				continue
			}
			name := strings.TrimSuffix(goString(message(manager, "displayNameAtPath:", nsString(path))), ".app")
			if name == "" {
				name = strings.TrimSuffix(filepath.Base(path), ".app")
			}
			bundleID := goString(message(bundle, "bundleIdentifier"))
			version := goString(message(bundle, "objectForInfoDictionaryKey:", nsString("CFBundleShortVersionString")))
			kind := "application"
			if strings.HasPrefix(path, "/System/") {
				kind = "system"
			}
			apps = append(apps, App{ID: applicationID(path), Name: name, Path: path, BundleID: bundleID, Version: version, Kind: kind})
		}
	})
	return apps, err
}

var imageMu sync.Mutex

func applicationIcon(path string) ([]byte, error) {
	// NSGraphicsContext's offscreen rendering is serialized. No UI main-thread
	// dispatch is involved, so calls cannot deadlock the HTTP preview server.
	imageMu.Lock()
	defer imageMu.Unlock()
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = real
	}
	var png []byte
	err := withCocoa(func() {
		workspace := message(class("NSWorkspace"), "sharedWorkspace")
		image := message(workspace, "iconForFile:", nsString(path))
		png = renderImagePNG(image, 256, 256)
	})
	if err != nil {
		return nil, err
	}
	if len(png) == 0 {
		return nil, errors.New("the application icon is unavailable")
	}
	return png, nil
}

// renderImagePNG must run inside withCocoa while imageMu is held.
func renderImagePNG(image objc.ID, width, height int) []byte {
	if image == 0 || width <= 0 || height <= 0 {
		return nil
	}
	rep := cocoa.initBitmap(message(class("NSBitmapImageRep"), "alloc"), objc.RegisterName("initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:"), 0, width, height, 8, 4, true, false, nsString("NSCalibratedRGBColorSpace"), 0, 0)
	if rep == 0 {
		return nil
	}
	defer message(rep, "release")
	rep = message(rep, "bitmapImageRepByRetaggingWithColorSpace:", message(class("NSColorSpace"), "sRGBColorSpace"))
	graphics := class("NSGraphicsContext")
	context := message(graphics, "graphicsContextWithBitmapImageRep:", rep)
	if context == 0 {
		return nil
	}
	message(graphics, "saveGraphicsState")
	defer message(graphics, "restoreGraphicsState")
	message(graphics, "setCurrentContext:", context)
	message(context, "setImageInterpolation:", 3)
	cocoa.drawImage(image, objc.RegisterName("drawInRect:fromRect:operation:fraction:"), nsRect{Size: nsSize{float64(width), float64(height)}}, nsRect{}, 2, 1)
	data := message(rep, "representationUsingType:properties:", 4, message(class("NSDictionary"), "dictionary"))
	length := int(message(data, "length"))
	pointer := objc.Send[unsafe.Pointer](data, objc.RegisterName("bytes"))
	if length > 0 && pointer != nil {
		return append([]byte(nil), unsafe.Slice((*byte)(pointer), length)...)
	}
	return nil
}

func openApplication(path string, reveal bool) error {
	args := []string{"--", path}
	if reveal {
		args = []string{"-R", "--", path}
	}
	output, err := exec.Command("/usr/bin/open", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("open application: %s: %w", strings.TrimSpace(string(output)), err)
	}
	return nil
}
