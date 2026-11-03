package launcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestRecentAppsAreOptInAndOnlySuccessfulLaunchesRecordHistory(t *testing.T) {
	s := testService(t)
	path := filepath.Join(t.TempDir(), "Example.app")
	makeBundle(t, path)
	s.scan = func() ([]App, error) { return []App{{ID: "a", Path: path}}, nil }
	failed := false
	opened := 0
	s.open = func(path string, reveal bool) error {
		opened++
		if failed {
			return errors.New("could not launch")
		}
		return nil
	}
	if err := s.Launch("a"); err != nil {
		t.Fatal(err)
	}
	if len(s.Preferences().Recent) != 0 {
		t.Fatal("usage recorded before opting in")
	}
	p := s.Preferences()
	p.ShowRecentApps = true
	if _, err := s.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	failed = true
	if err := s.Launch("a"); err == nil || len(s.Preferences().Recent) != 0 {
		t.Fatal("failed launch recorded usage")
	}
	failed = false
	if err := s.Reveal("a"); err != nil || len(s.Preferences().Recent) != 0 {
		t.Fatal("Finder reveal recorded usage", err)
	}
	if err := s.Launch("a"); err != nil || !reflect.DeepEqual(s.Preferences().Recent, []string{"a"}) {
		t.Fatal("successful launch missing history", err, s.Preferences())
	}
	if opened != 4 {
		t.Fatal("unexpected application open count", opened)
	}
}

func TestRecentHistoryIsBoundedMovesToFrontAndSurvivesRestart(t *testing.T) {
	s := testService(t)
	p := s.Preferences()
	p.ShowRecentApps = true
	if _, err := s.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	for i := range 30 {
		s.recordLaunch(fmt.Sprintf("app-%d", i))
	}
	s.recordLaunch("app-12")
	p = s.Preferences()
	if len(p.Recent) != 24 || p.Recent[0] != "app-12" || p.Recent[1] != "app-29" || slices.Contains(p.Recent, "app-0") {
		t.Fatal(p.Recent)
	}
	p.Recent[0] = "mutated"
	reopened, err := New(s.configPath)
	if err != nil || reopened.Preferences().Recent[0] != "app-12" {
		t.Fatal("history lost after restart", err)
	}
}

func TestSuccessfulLaunchStillSucceedsWhenRecentHistoryCannotBeSaved(t *testing.T) {
	s := testService(t)
	path := filepath.Join(t.TempDir(), "Example.app")
	makeBundle(t, path)
	s.scan = func() ([]App, error) { return []App{{ID: "a", Path: path}}, nil }
	s.open = func(string, bool) error { return nil }
	p := s.Preferences()
	p.ShowRecentApps = true
	if _, err := s.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.configPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.configPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.Launch("a"); err != nil {
		t.Fatal("history persistence failure turned success into launch failure", err)
	}
}

func TestRecentCategoryIsDynamicExcludesHiddenAppsAndKeepsPageMemory(t *testing.T) {
	p := defaultPreferences()
	if slices.ContainsFunc(VisibleCategories(p), func(c Category) bool { return c.ID == "recent" }) {
		t.Fatal("recent shown by default")
	}
	p.ShowRecentApps = true
	p.Language = "zh-CN"
	p.Recent = []string{"a", "b"}
	p.Hidden = []string{"b"}
	p.LastCategory = "recent"
	p.PageMemory["recent"] = 2
	p = normalizePreferences(p)
	categories := VisibleCategories(p)
	recent := categories[len(categories)-1]
	if recent.ID != "recent" || recent.Name != "最近使用" || p.LastCategory != "recent" || p.PageMemory["recent"] != 2 {
		t.Fatal(recent, p)
	}
	if !CategoryContains(recent, App{ID: "a"}, p) || CategoryContains(recent, App{ID: "b"}, p) || CategoryContains(recent, App{ID: "c"}, p) {
		t.Fatal("recent membership includes hidden or unused applications")
	}
	s := testService(t)
	if _, err := s.SavePreferences(p); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPage("recent", 3); err != nil {
		t.Fatal(err)
	}
	stale := defaultPreferences()
	if _, err := s.SaveManagementPreferences(stale); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Preferences().Recent, []string{"a", "b"}) || s.Preferences().PageMemory["recent"] != 3 {
		t.Fatal("stale settings overwrote usage or recent page", s.Preferences())
	}
}

func TestSuggestedCategoriesUseMetadataAndPreserveExistingGroups(t *testing.T) {
	p := defaultPreferences()
	p.Language = "zh-CN"
	p.Hidden = []string{"hidden"}
	state := State{Preferences: p, Apps: []App{
		{ID: "dev-a", Name: "Other", Category: "public.app-category.developer-tools"},
		{ID: "dev-b", Name: "Different", Category: "public.app-category.developer-tools"},
		{ID: "creative-a", Category: "public.app-category.photography"},
		{ID: "creative-b", Category: "public.app-category.video"},
		{ID: "hidden", Category: "public.app-category.developer-tools"},
		{ID: "system", Kind: "system", Category: "public.app-category.developer-tools"},
		{ID: "system-path", Path: "/System/Applications/Tool.app", Category: "public.app-category.developer-tools"},
		{ID: "unclassified", Name: "Developer Tools"},
		{ID: "one-utility", Category: "public.app-category.utilities"},
		{ID: "unknown", Category: "public.app-category.unknown"},
	}}
	got := SuggestedCategories(state)
	if len(got) != 2 || got[0].ID != "suggested-development" || got[0].Name != "开发" || !reflect.DeepEqual(got[0].AppIDs, []string{"dev-a", "dev-b"}) || got[1].ID != "suggested-creativity" {
		t.Fatal(got)
	}
	state.Preferences.Categories = append(state.Preferences.Categories, Category{ID: "suggested-development", Name: "My Renamed Group", AppIDs: []string{"custom"}, Visible: false})
	got = SuggestedCategoriesForLocale(state, "en")
	if len(got) != 1 || got[0].Name != "Creativity" {
		t.Fatal("existing edited suggestion was recreated", got)
	}
	s := testService(t)
	s.scan = func() ([]App, error) { return state.Apps, nil }
	if _, err := s.SavePreferences(state.Preferences); err != nil {
		t.Fatal(err)
	}
	result, err := s.AddSuggestedCategories()
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(result.Preferences.Categories, func(c Category) bool { return c.ID == "suggested-development" })
	if index < 0 || result.Preferences.Categories[index].Name != "My Renamed Group" || result.Preferences.Categories[index].Visible || !reflect.DeepEqual(result.Preferences.Categories[index].AppIDs, []string{"custom"}) {
		t.Fatal("suggestion application overwrote manual group", result.Preferences.Categories)
	}
}
