package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func testService(t *testing.T) *Service {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "settings", "preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.scan = func() ([]App, error) {
		return []App{{ID: "a", Name: "Alpha", Path: "/Applications/Alpha.app"}, {ID: "b", Name: "Beta", Path: "/Applications/Beta.app"}}, nil
	}
	return s
}

func TestPreferencesRoundTripAndDefensiveCopies(t *testing.T) {
	s := testService(t)
	preferences := Preferences{Hidden: []string{"b", "b", ""}, Order: []string{"b", "a", "b", "temporarily-uninstalled"}, Favorites: []string{"a"}, IconSize: 999}
	state, err := s.SavePreferences(preferences)
	if err != nil {
		t.Fatal(err)
	}
	if state.Preferences.IconSize != 112 {
		t.Fatal("icon size not clamped", state.Preferences.IconSize)
	}
	if !reflect.DeepEqual(state.Preferences.Order, []string{"b", "a", "temporarily-uninstalled"}) {
		t.Fatal(state.Preferences.Order)
	}
	if !reflect.DeepEqual(state.Preferences.Hidden, []string{"b"}) {
		t.Fatal(state.Preferences.Hidden)
	}
	state.Preferences.Order[0] = "changed"
	state.Apps[0].Path = "changed"
	state, _ = s.State()
	if state.Preferences.Order[0] != "b" || state.Apps[0].Path == "changed" {
		t.Fatal("state exposes internal memory")
	}
	other, err := New(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(other.preferences, state.Preferences) {
		t.Fatal("preferences lost after reopen")
	}
	info, err := os.Stat(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatal("preferences should be private", info.Mode())
	}
}

func TestFailedSaveDoesNotPublishPreferences(t *testing.T) {
	s := testService(t)
	if err := os.MkdirAll(s.configPath, 0700); err != nil {
		t.Fatal(err)
	}
	_, err := s.SavePreferences(Preferences{IconSize: 48, Hidden: []string{"a"}})
	if err == nil {
		t.Fatal("saving over directory should fail")
	}
	state, _ := s.State()
	if len(state.Preferences.Hidden) != 0 || state.Preferences.IconSize != 80 {
		t.Fatal("failed save changed state")
	}
}

func TestDefaultsAndMalformedPreferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := os.WriteFile(path, []byte("{broken"), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.preferences.IconSize != 80 {
		t.Fatal(s.preferences)
	}
	data, err := json.Marshal(s.preferences)
	if err != nil {
		t.Fatal(err)
	}
	var serialized map[string]json.RawMessage
	if err := json.Unmarshal(data, &serialized); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"hidden", "order", "favorites"} {
		if string(serialized[key]) != "[]" {
			t.Fatalf("%s must serialize as an empty array: %s", key, data)
		}
	}
	if got, _ := os.ReadFile(path); string(got) != "{broken" {
		t.Fatal("corrupted file was not preserved")
	}
}

func TestMalformedPreferencesRemainRecoverableAfterSaving(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preferences.json")
	broken := []byte(`{"order":["important-app"],"categories":[`)
	if err := os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	previous := filepath.Join(dir, "preferences.json.corrupt-existing")
	if err := os.WriteFile(previous, []byte("earlier recovery"), 0600); err != nil {
		t.Fatal(err)
	}
	// Reopening before a save must create distinct recovery files, never
	// overwrite an earlier recovery attempt.
	var s *Service
	for range 2 {
		var err error
		s, err = New(path)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetLastCategory("favorites"); err != nil {
		t.Fatal(err)
	}
	backups, err := filepath.Glob(path + ".corrupt-*")
	if err != nil || len(backups) != 3 {
		t.Fatalf("expected two distinct backups plus existing recovery, got %v: %v", backups, err)
	}
	for _, backup := range backups {
		data, err := os.ReadFile(backup)
		if err != nil {
			t.Fatal(err)
		}
		if backup == previous {
			if string(data) != "earlier recovery" {
				t.Fatal("existing recovery was overwritten")
			}
			continue
		}
		if string(data) != string(broken) {
			t.Fatalf("original malformed preferences lost: %q", data)
		}
		info, err := os.Stat(backup)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("recovery must be private: %v, %v", info, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || !json.Valid(data) {
		t.Fatalf("new preferences were not saved: %q, %v", data, err)
	}
}

func TestMalformedPreferencesBackupFailureDoesNotOpenWritableDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "preferences.json")
	broken := []byte("{broken")
	if err := os.WriteFile(path, broken, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	// Some test runners run as root and bypass directory permissions.
	probe, err := os.CreateTemp(dir, ".permission-probe-*")
	if err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("test runner can write to a read-only directory")
	}
	if s, err := New(path); err == nil || s != nil {
		t.Fatalf("backup failure opened writable defaults: service=%v, err=%v", s, err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != string(broken) {
		t.Fatalf("backup failure changed original preferences: %q, %v", data, err)
	}
}

func TestUnknownApplicationCannotLaunchOrRevealArbitraryPaths(t *testing.T) {
	s := testService(t)
	for _, id := range []string{"", "/Applications/Alpha.app", "../../etc/passwd", "unknown"} {
		if err := s.Launch(id); err == nil {
			t.Errorf("Launch accepted %q", id)
		}
		if err := s.Reveal(id); err == nil {
			t.Errorf("Reveal accepted %q", id)
		}
		if _, err := s.Icon(id); err == nil {
			t.Errorf("Icon accepted %q", id)
		}
	}
}

func makeBundle(t *testing.T, path string) {
	t.Helper()
	contents := filepath.Join(path, "Contents")
	if err := os.MkdirAll(contents, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte("<?xml version=\"1.0\"?><plist version=\"1.0\"><dict><key>CFBundleIdentifier</key><string>test.example</string></dict></plist>"), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoveryRecursesSkipsHelpersAndDeduplicatesSymlinks(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "Alpha.app")
	b := filepath.Join(root, "Developer", "Tools", "Beta.app")
	makeBundle(t, a)
	makeBundle(t, b)
	makeBundle(t, filepath.Join(a, "Contents", "Helpers", "Nested.app"))
	makeBundle(t, filepath.Join(root, ".hidden", "Hidden.app"))
	if err := os.Mkdir(filepath.Join(root, "Empty.app"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "cycle")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(a, filepath.Join(root, "Alias.app")); err != nil {
		t.Fatal(err)
	}
	paths, err := discoverBundles([]string{root, root, filepath.Join(root, "missing")})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 {
		t.Fatal("nested helpers, duplicates, hidden or abandoned apps found", paths)
	}
	seen := map[string]bool{}
	for _, path := range paths {
		real, _ := filepath.EvalSymlinks(path)
		seen[real] = true
	}
	realA, _ := filepath.EvalSymlinks(a)
	realB, _ := filepath.EvalSymlinks(b)
	if !seen[realA] || !seen[realB] {
		t.Fatal(paths)
	}
}

func TestApplicationIdentitySurvivesNewSymlinkAliases(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "Example.app")
	alias := filepath.Join(root, "Earlier.app")
	makeBundle(t, app)
	id := applicationID(app)
	if err := os.Symlink(app, alias); err != nil {
		t.Fatal(err)
	}
	if got := applicationID(alias); got != id {
		t.Fatalf("alias changed application identity: %s != %s", got, id)
	}
}

func TestLaunchRejectsPreviouslyCataloguedBundleReplacedByFile(t *testing.T) {
	s := testService(t)
	path := filepath.Join(t.TempDir(), "Example.app")
	makeBundle(t, path)
	s.scan = func() ([]App, error) { return []App{{ID: "app", Path: path}}, nil }
	if _, err := s.State(); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("ordinary document"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Launch("app"); err == nil {
		t.Fatal("launch accepted a document that replaced a catalogued application")
	}
}
