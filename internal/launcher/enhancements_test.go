package launcher

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"
)

func TestLegacyPreferencesReceiveInteractionDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := os.WriteFile(path, []byte(`{"iconSize":96,"loopPages":true,"dismissOnEmptyClick":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Preferences()
	if p.SearchScope != "all" || p.SearchRanking != "relevance" || !p.RememberPage || !p.RememberCategory || !p.EscapeClearsSearch || !p.TabCyclesCategories {
		t.Fatal(p)
	}
	if !p.LoopPages || p.DismissOnEmptyClick {
		t.Fatal("explicit saved choices were replaced", p)
	}
}

func TestPageMemoryPreservesLatestNavigationAndDefensiveCopies(t *testing.T) {
	s := testService(t)
	stale := s.Preferences()
	if err := s.SetPage("favorites", 3); err != nil {
		t.Fatal(err)
	}
	stale.Language = "en"
	state, err := s.SaveManagementPreferences(stale)
	if err != nil {
		t.Fatal(err)
	}
	if state.Preferences.PageMemory["favorites"] != 3 || state.Preferences.Language != "en" {
		t.Fatal(state.Preferences)
	}
	state.Preferences.PageMemory["favorites"] = 99
	if s.Preferences().PageMemory["favorites"] != 3 {
		t.Fatal("page map escaped")
	}
	if err := s.SetPage("removed-section", 2); err == nil {
		t.Fatal("remembered an unknown section")
	}
}

func TestStableIdentityMigratesReferencesAndSurvivesMoveAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	oldPath := "/Applications/Example.app"
	legacyID := applicationID(oldPath)
	s, _ := New(path)
	p := s.Preferences()
	p.Hidden, p.Favorites, p.Order = []string{legacyID}, []string{legacyID}, []string{legacyID}
	p.Recent = []string{legacyID}
	p.Aliases[legacyID] = "My Tool"
	p.Categories = append(p.Categories, Category{ID: "work", Name: "Work", AppIDs: []string{legacyID}, Visible: true})
	if _, err := s.savePreferencesLocked(p); err != nil {
		t.Fatal(err)
	}
	currentPath := oldPath
	s.scan = func() ([]App, error) {
		return []App{{ID: applicationID(currentPath), Name: "Example", Path: currentPath, BundleID: "com.example.tool"}}, nil
	}
	state, err := s.State()
	if err != nil {
		t.Fatal(err)
	}
	stable := state.Apps[0].ID
	if stable == legacyID || state.Preferences.Hidden[0] != stable || state.Preferences.Favorites[0] != stable || state.Preferences.Order[0] != stable || state.Preferences.Aliases[stable] != "My Tool" || state.Preferences.Recent[0] != stable {
		t.Fatal("legacy references did not migrate", state)
	}
	if state.Preferences.Categories[len(state.Preferences.Categories)-1].AppIDs[0] != stable {
		t.Fatal("category reference did not migrate")
	}
	currentPath = "/Applications/Tools/Renamed.app"
	state, err = s.Refresh()
	if err != nil || state.Apps[0].ID != stable {
		t.Fatal("move changed identity", state, err)
	}
	reopened, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	reopened.scan = s.scan
	state, err = reopened.State()
	if err != nil || state.Apps[0].ID != stable || !slices.Contains(state.Preferences.Hidden, stable) {
		t.Fatal("restart lost migrated reference", state, err)
	}
}

func TestDuplicateBundlesKeepDistinctIdentitiesAndPrimaryCopy(t *testing.T) {
	s := testService(t)
	paths := []string{"/Applications/Example.app", "/Users/test/Applications/Example.app"}
	s.scan = func() ([]App, error) {
		apps := make([]App, len(paths))
		for i, path := range paths {
			apps[i] = App{ID: applicationID(path), Path: path, BundleID: "com.example.tool"}
		}
		return apps, nil
	}
	state, err := s.State()
	if err != nil {
		t.Fatal(err)
	}
	primary, copyID := state.Apps[0].ID, state.Apps[1].ID
	if primary == copyID {
		t.Fatal("duplicate copies collapsed")
	}
	paths = []string{"/Applications/AAA Backup.app", "/Applications/Example.app", "/Users/test/Applications/Example.app"}
	state, err = s.Refresh()
	if err != nil || state.Apps[1].ID != primary || state.Apps[2].ID != copyID || state.Apps[0].ID == primary {
		t.Fatal("new copy took primary identity", state, err)
	}
	reopened, err := New(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	reopened.scan = s.scan
	state, err = reopened.State()
	if err != nil || state.Apps[1].ID != primary || state.Apps[0].ID == primary {
		t.Fatal("restart transferred primary identity to backup", state, err)
	}
}

func TestFreshCatalogSkipsRescanAndRefreshKeepsOnlyCurrentIcons(t *testing.T) {
	s := testService(t)
	calls := 0
	version := "1"
	s.scan = func() ([]App, error) {
		calls++
		return []App{{ID: "a", Path: "/Applications/Alpha.app", Version: version}, {ID: "b", Path: "/Applications/Beta.app", Version: "1"}}, nil
	}
	state, err := s.RefreshIfStale()
	if err != nil {
		t.Fatal(err)
	}
	a, b := iconCacheKey(state.Apps[0]), iconCacheKey(state.Apps[1])
	s.icons[a], s.icons[b] = []byte("alpha"), []byte("beta")
	if _, err = s.RefreshIfStale(); err != nil || calls != 1 {
		t.Fatal("fresh catalog was rescanned", calls, err)
	}
	version = "2"
	if _, err = s.Refresh(); err != nil || calls != 2 {
		t.Fatal("explicit refresh did not rescan", calls, err)
	}
	if _, ok := s.icons[a]; ok || string(s.icons[b]) != "beta" {
		t.Fatal("incremental icon invalidation failed", s.icons)
	}
	s.lastScan = time.Now().Add(-time.Minute)
	if _, err = s.RefreshIfStale(); err != nil || calls != 3 {
		t.Fatal("stale catalog not refreshed", calls, err)
	}
}

func TestAtomicAppActionsPreserveOtherEditsAndFullOrder(t *testing.T) {
	s := testService(t)
	p := s.Preferences()
	p.Language = "en"
	p.Categories = append(p.Categories, Category{ID: "work", Name: "Work", Visible: true})
	if _, err := s.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	for _, action := range []struct{ action, id, target string }{{"favorite", "a", ""}, {"hide", "b", ""}, {"category", "a", "work"}, {"reorder", "b", "a"}} {
		if _, err := s.ApplyAppAction(action.action, action.id, action.target); err != nil {
			t.Fatal(err)
		}
	}
	p = s.Preferences()
	if p.Language != "en" || !reflect.DeepEqual(p.Order, []string{"b", "a"}) || !slices.Contains(p.Favorites, "a") || !slices.Contains(p.Hidden, "b") {
		t.Fatal(p)
	}
	if !slices.Contains(p.Categories[len(p.Categories)-1].AppIDs, "a") {
		t.Fatal("membership missing", p)
	}
	if _, err := s.ApplyAppAction("remove-category", "a", "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyAppAction("restore", "b", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyAppAction("favorite", "a", ""); err != nil {
		t.Fatal(err)
	}
	p = s.Preferences()
	if len(p.Hidden) != 0 || len(p.Favorites) != 0 || len(p.Categories[len(p.Categories)-1].AppIDs) != 0 {
		t.Fatal(p)
	}
	if _, err := s.ApplyAppAction("hide", "unknown", ""); err == nil {
		t.Fatal("accepted unknown app")
	}
	if _, err := s.ApplyAppAction("category", "a", "all"); err == nil {
		t.Fatal("accepted built-in category membership edit")
	}
}
