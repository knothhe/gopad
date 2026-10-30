// Package updatearchive adds ZIP input to the pinned MyGo updater. MyGo still
// verifies the downloaded bytes and owns staging, application exit and replacement.
package updatearchive

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

const MaxSize = 1 << 30
const maxEntries = 100000
const bundle = "gopad.app"

// IsZIP inspects the downloaded file without moving its read offset. Other
// formats continue through MyGo's original tar.gz extractor.
func IsZIP(f *os.File) bool {
	var magic [4]byte
	n, _ := f.ReadAt(magic[:], 0)
	return n == 4 && (string(magic[:]) == "PK\x03\x04" || string(magic[:]) == "PK\x05\x06")
}

type entry struct {
	file       *zip.File
	name, link string
	mode       fs.FileMode
}

// ExtractZIP accepts only one gopad.app, with bounded files, directories and
// relative links inside that bundle. AppleDouble resource metadata from ditto
// is ignored; app files, their Unix permissions and code signatures are retained.
func ExtractZIP(f *os.File, dir string) error {
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if info.Size() <= 0 || info.Size() > MaxSize {
		return fmt.Errorf("invalid ZIP size")
	}
	zr, err := zip.NewReader(f, info.Size())
	if err != nil {
		return err
	}
	if len(zr.File) == 0 || len(zr.File) > maxEntries {
		return fmt.Errorf("invalid ZIP entry count")
	}
	entries := make([]entry, 0, len(zr.File))
	names := make(map[string]fs.FileMode)
	spellings := make(map[string]string)
	canonical := func(value string) string { return norm.NFC.String(cases.Fold().String(value)) }
	var total uint64
	for _, zf := range zr.File {
		name := strings.TrimSuffix(zf.Name, "/")
		if name == "." || !fs.ValidPath(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\:\x00") {
			return fmt.Errorf("invalid path in ZIP: %q", zf.Name)
		}
		mode := zf.Mode()
		if !mode.IsRegular() && !mode.IsDir() && mode&os.ModeSymlink == 0 {
			return fmt.Errorf("unsupported ZIP entry: %q", name)
		}
		if _, found := names[canonical(name)]; found {
			return fmt.Errorf("duplicate ZIP entry: %q", name)
		}
		names[canonical(name)] = mode
		for part := name; part != "."; part = path.Dir(part) {
			key := canonical(part)
			if previous, exists := spellings[key]; exists && previous != part {
				return fmt.Errorf("ambiguous ZIP path spelling: %q", name)
			}
			spellings[key] = part
		}
		if zf.UncompressedSize64 > MaxSize-total {
			return fmt.Errorf("ZIP expands beyond size limit")
		}
		total += zf.UncompressedSize64
		if name == "__MACOSX" || strings.HasPrefix(name, "__MACOSX/") {
			if mode&os.ModeSymlink != 0 {
				return fmt.Errorf("invalid AppleDouble metadata")
			}
			continue
		}
		if name != bundle && !strings.HasPrefix(name, bundle+"/") {
			return fmt.Errorf("ZIP contains another application or file: %q", name)
		}
		if name == bundle && !mode.IsDir() {
			return fmt.Errorf("ZIP bundle root is not a directory")
		}
		if strings.HasPrefix(path.Base(name), "._") {
			if !mode.IsRegular() {
				return fmt.Errorf("invalid AppleDouble metadata")
			}
			continue
		}
		e := entry{file: zf, name: name, mode: mode}
		if mode&os.ModeSymlink != 0 {
			if zf.UncompressedSize64 == 0 || zf.UncompressedSize64 > 4096 {
				return fmt.Errorf("invalid ZIP link")
			}
			r, err := zf.Open()
			if err != nil {
				return err
			}
			data, readErr := io.ReadAll(io.LimitReader(r, 4097))
			r.Close()
			if readErr != nil {
				return readErr
			}
			e.link = string(data)
			resolved := path.Join(path.Dir(name), e.link)
			if strings.ContainsAny(e.link, "\\\x00:") || path.IsAbs(e.link) || (resolved != bundle && !strings.HasPrefix(resolved, bundle+"/")) {
				return fmt.Errorf("ZIP link points outside the application: %q", name)
			}
		}
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		return fmt.Errorf("ZIP lacks an application")
	}
	// Check every parent before extracting anything, independent of archive order.
	for _, e := range entries {
		for parent := path.Dir(e.name); parent != "."; parent = path.Dir(parent) {
			if mode, ok := names[canonical(parent)]; ok && !mode.IsDir() {
				return fmt.Errorf("ZIP entry has a non-directory parent: %q", e.name)
			}
		}
	}
	if fi, err := os.Lstat(dir); err == nil {
		if !fi.IsDir() || fi.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("unsafe ZIP destination")
		}
		children, err := os.ReadDir(dir)
		if err != nil {
			return err
		}
		if len(children) != 0 {
			return fmt.Errorf("ZIP destination must be empty")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, e := range entries {
		target := filepath.Join(dir, filepath.FromSlash(e.name))
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if e.mode.IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return err
			}
			continue
		}
		if e.mode&os.ModeSymlink != 0 {
			if err := os.Symlink(e.link, target); err != nil {
				return err
			}
			continue
		}
		r, err := e.file.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, e.mode.Perm())
		if err != nil {
			r.Close()
			return err
		}
		n, copyErr := io.Copy(out, io.LimitReader(r, int64(e.file.UncompressedSize64)+1))
		closeErr := out.Close()
		r.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if uint64(n) != e.file.UncompressedSize64 {
			return fmt.Errorf("ZIP entry size mismatch")
		}
		if err := os.Chmod(target, e.mode.Perm()); err != nil {
			return err
		}
	}
	root, err := filepath.EvalSymlinks(filepath.Join(dir, bundle))
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.mode&os.ModeSymlink == 0 {
			continue
		}
		resolved, err := filepath.EvalSymlinks(filepath.Join(dir, filepath.FromSlash(e.name)))
		if err != nil {
			return fmt.Errorf("invalid ZIP link %q: %w", e.name, err)
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("ZIP link resolves outside the application: %q", e.name)
		}
	}
	// Child directories must receive their final modes before restrictive parents.
	sort.SliceStable(entries, func(i, j int) bool { return strings.Count(entries[i].name, "/") > strings.Count(entries[j].name, "/") })
	for _, e := range entries {
		if e.mode.IsDir() {
			if err := os.Chmod(filepath.Join(dir, filepath.FromSlash(e.name)), e.mode.Perm()); err != nil {
				return err
			}
		}
	}
	return nil
}
