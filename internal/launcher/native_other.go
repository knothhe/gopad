//go:build !darwin

package launcher

import "errors"

func describeApplications(paths []string) ([]App, error) {
	return nil, errors.New("gopad requires macOS")
}
func applicationIcon(path string) ([]byte, error)    { return nil, errors.New("gopad requires macOS") }
func openApplication(path string, reveal bool) error { return errors.New("gopad requires macOS") }
func Wallpaper() ([]byte, error)                     { return nil, errors.New("gopad requires macOS") }
func WallpaperForDisplay(int64) ([]byte, error)      { return nil, errors.New("gopad requires macOS") }
