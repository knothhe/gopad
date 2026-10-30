//go:build darwin

package launcher

import (
	"bytes"
	"image/png"
	"testing"
)

func TestNativeDiscoveryAndIconWithoutApplicationLoop(t *testing.T) {
	apps, err := ScanApplications()
	if err != nil {
		t.Fatal(err)
	}
	if len(apps) == 0 {
		t.Fatal("no installed applications found")
	}
	var finder App
	for _, app := range apps {
		if app.BundleID == "com.apple.finder" {
			finder = app
			break
		}
	}
	if finder.ID == "" {
		t.Fatal("Finder is missing from the application catalog")
	}
	data, err := applicationIcon(finder.Path)
	if err != nil {
		t.Fatal(err)
	}
	image, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if image.Width != 256 || image.Height != 256 {
		t.Fatal("unexpected icon size", image)
	}
	if finder.Kind != "system" || finder.Name == "" {
		t.Fatal("missing native app metadata", finder)
	}
}

func TestNativeWallpaperPNGWhenDesktopStillIsAvailable(t *testing.T) {
	data, err := Wallpaper()
	if err != nil {
		t.Skipf("desktop has no readable still image: %v", err)
	}
	config, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > 2048 || config.Height > 2048 {
		t.Fatalf("wallpaper texture is not bounded: %dx%d", config.Width, config.Height)
	}
	t.Logf("wallpaper PNG: %dx%d, %d bytes", config.Width, config.Height, len(data))
}
