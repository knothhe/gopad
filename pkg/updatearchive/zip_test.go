package updatearchive

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureEntry struct {
	name, data string
	mode       fs.FileMode
}

func fixtureZIP(t *testing.T, entries []fixtureEntry) []byte {
	t.Helper()
	var data bytes.Buffer
	zw := zip.NewWriter(&data)
	for _, entry := range entries {
		h := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		h.SetMode(entry.mode)
		w, err := zw.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(entry.data)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func extractFixture(t *testing.T, data []byte) (string, error) {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "app.zip")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(file)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !IsZIP(f) {
		t.Fatal("ZIP detection failed")
	}
	out := filepath.Join(dir, "unpacked")
	return out, ExtractZIP(f, out)
}

func TestExtractZIPPreservesExecutableAndInternalLinks(t *testing.T) {
	out, err := extractFixture(t, fixtureZIP(t, []fixtureEntry{
		{"gopad.app/", "", fs.ModeDir | 0755},
		{"gopad.app/Contents/MacOS/gopad", "executable", 0755},
		{"gopad.app/Contents/Frameworks/Current", "Versions/A", fs.ModeSymlink | 0777},
		{"gopad.app/Contents/Frameworks/Versions/A/library", "library", 0644},
		{"__MACOSX/gopad.app/._Contents", "AppleDouble", 0644},
		{"gopad.app/Contents/._resource", "AppleDouble", 0644},
	}))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(out, "gopad.app/Contents/MacOS/gopad"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("executable permissions %v", info.Mode())
	}
	data, err := os.ReadFile(filepath.Join(out, "gopad.app/Contents/Frameworks/Current/library"))
	if err != nil || string(data) != "library" {
		t.Fatalf("internal link: %s, %v", data, err)
	}
	children, err := os.ReadDir(out)
	if err != nil || len(children) != 1 || children[0].Name() != "gopad.app" {
		t.Fatalf("unexpected root entries: %v, %v", children, err)
	}
	if _, err := os.Stat(filepath.Join(out, "gopad.app/Contents/._resource")); !os.IsNotExist(err) {
		t.Fatal("AppleDouble was extracted")
	}
}

func TestExtractZIPRejectsUnsafeEntries(t *testing.T) {
	for _, test := range []struct {
		name    string
		entries []fixtureEntry
	}{
		{"traversal", []fixtureEntry{{"gopad.app/../outside", "x", 0644}}},
		{"absolute", []fixtureEntry{{"/gopad.app/file", "x", 0644}}},
		{"windows", []fixtureEntry{{"gopad.app/C:\\outside", "x", 0644}}},
		{"another bundle", []fixtureEntry{{"other.app/file", "x", 0644}}},
		{"duplicate", []fixtureEntry{{"gopad.app/file", "x", 0644}, {"gopad.app/file", "y", 0644}}},
		{"link escape", []fixtureEntry{{"gopad.app/link", "../../outside", fs.ModeSymlink | 0777}}},
		{"absolute link", []fixtureEntry{{"gopad.app/link", "/tmp/outside", fs.ModeSymlink | 0777}}},
		{"symlink parent", []fixtureEntry{{"gopad.app/link/file", "x", 0644}, {"gopad.app/link", "Contents", fs.ModeSymlink | 0777}}},
		{"file parent", []fixtureEntry{{"gopad.app/file", "x", 0644}, {"gopad.app/file/child", "y", 0644}}},
		{"metadata symlink", []fixtureEntry{{"__MACOSX/link", "gopad.app", fs.ModeSymlink | 0777}}},
		{"bundle symlink", []fixtureEntry{{"gopad.app", "inside", fs.ModeSymlink | 0777}}},
		{"case collision", []fixtureEntry{{"gopad.app/File", "x", 0644}, {"gopad.app/file", "y", 0644}}},
		{"unicode collision", []fixtureEntry{{"gopad.app/caf\u00e9", "x", 0644}, {"gopad.app/cafe\u0301", "y", 0644}}},
		{"parent case collision", []fixtureEntry{{"gopad.app/Sub/file", "x", 0644}, {"gopad.app/sub/child", "y", 0644}}},
		{"dangling link", []fixtureEntry{{"gopad.app/link", "absent", fs.ModeSymlink | 0777}}},
		{"link cycle", []fixtureEntry{{"gopad.app/a", "b", fs.ModeSymlink | 0777}, {"gopad.app/b", "a", fs.ModeSymlink | 0777}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := extractFixture(t, fixtureZIP(t, test.entries)); err == nil {
				t.Fatal("accepted unsafe ZIP")
			}
		})
	}
}

func TestExtractZIPRejectsSymlinkChainEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(dir, "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(dir, "app.zip")
	data := fixtureZIP(t, []fixtureEntry{
		{"gopad.app/a", ".", fs.ModeSymlink | 0777},
		{"gopad.app/b", "a/../outside", fs.ModeSymlink | 0777},
	})
	if err := os.WriteFile(archive, data, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := ExtractZIP(f, filepath.Join(dir, "unpacked")); err == nil {
		t.Fatal("accepted link chain outside the application")
	}
}

func TestExtractZIPRejectsExpansionAndCRCFailure(t *testing.T) {
	var data bytes.Buffer
	zw := zip.NewWriter(&data)
	h := &zip.FileHeader{Name: "gopad.app/bomb", Method: zip.Store, UncompressedSize64: MaxSize + 1}
	h.SetMode(0644)
	if _, err := zw.CreateRaw(h); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := extractFixture(t, data.Bytes()); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("expansion limit: %v", err)
	}
	valid := fixtureZIP(t, []fixtureEntry{{"gopad.app/file", "unique payload", 0644}})
	corrupt := bytes.Replace(valid, []byte("unique payload"), []byte("broken payload"), 1)
	if _, err := extractFixture(t, corrupt); err == nil {
		t.Fatal("accepted corrupted CRC")
	}
}

func TestExtractZIPRefusesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "app-*.zip")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.Write(fixtureZIP(t, []fixtureEntry{{"gopad.app/file", "x", 0644}}))
	if err := ExtractZIP(f, dir); err == nil {
		t.Fatal("accepted nonempty destination")
	}
	link := filepath.Join(dir, "destination")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if err := ExtractZIP(f, link); err == nil {
		t.Fatal("accepted symlink destination")
	}
}
