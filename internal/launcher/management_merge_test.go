package launcher

import (
	"os"
	"reflect"
	"slices"
	"testing"
)

func TestManagementMergePreservesConcurrentNativeEdits(t *testing.T) {
	s := testService(t)
	p := s.Preferences()
	p.Favorites = []string{"b"}
	p.Categories = append(p.Categories, Category{ID: "work", Name: "Work", Visible: true, AppIDs: []string{"a"}})
	state, err := s.SavePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	base := state.Preferences
	next := clonePreferences(base)
	next.IconSize = 96
	next.Hidden = []string{"a"}
	next.Favorites = nil // Remove b, while a is added independently below.
	for i := range next.Categories {
		if next.Categories[i].ID == "work" {
			next.Categories[i].Name = "Design"
			next.Categories[i].AppIDs = []string{"c"}
		}
	}
	for _, action := range []struct{ action, id, target string }{{"favorite", "a", ""}, {"hide", "b", ""}, {"category", "b", "work"}, {"reorder", "b", "a"}} {
		if _, err := s.ApplyAppAction(action.action, action.id, action.target); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetLastCategory("favorites"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPage("favorites", 3); err != nil {
		t.Fatal(err)
	}
	state, err = s.SaveManagementPreferencesWithBase(next, base)
	if err != nil {
		t.Fatal(err)
	}
	got := state.Preferences
	if got.IconSize != 96 || !reflect.DeepEqual(got.Hidden, []string{"b", "a"}) || !reflect.DeepEqual(got.Favorites, []string{"a"}) || !reflect.DeepEqual(got.Order, []string{"b", "a"}) {
		t.Fatal("stale settings overwrote concurrent native edit", got)
	}
	if got.LastCategory != "favorites" || got.PageMemory["favorites"] != 3 {
		t.Fatal("stale settings replaced navigation memory", got)
	}
	work := got.Categories[slices.IndexFunc(got.Categories, func(c Category) bool { return c.ID == "work" })]
	if work.Name != "Design" || !reflect.DeepEqual(work.AppIDs, []string{"b", "c"}) {
		t.Fatal("category edits did not merge independently", work)
	}
}

func TestManagementMergeUntouchedFieldsPreserveCurrentValues(t *testing.T) {
	base := defaultPreferences()
	current := clonePreferences(base)
	current.IconSize = 112
	current.Language = "en"
	current.ShowMenuBarIcon = true
	current.SearchScope = "current"
	current.SearchRanking = "manual"
	current.LoopPages = true
	current.RememberPage = false
	current.DismissOnEmptyClick = false
	current.EscapeClearsSearch = false
	current.RememberCategory = false
	current.TabCyclesCategories = false
	current.Shortcut = "Ctrl+Alt+G"
	current.LaunchAtLogin = true
	current.ShowRecentApps = true
	current.ShowCategoryFolders = true
	current.Recent = []string{"recent-app"}
	current.Hidden = []string{"hidden"}
	current.Favorites = []string{"favorite"}
	current.Order = []string{"new", "favorite"}
	current.LastCategory = "favorites"
	current.PageMemory["favorites"] = 2
	current.AppLocations["bundle:identity"] = "/Applications/Current.app"
	current.Aliases["favorite"] = "My App"
	current.Categories = append(current.Categories, Category{ID: "new", Name: "New", Visible: true})
	got := MergeManagementPreferences(current, base, base)
	want := normalizePreferences(current)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("untouched values changed\ngot: %#v\nwant:%#v", got, want)
	}
	got.Aliases["favorite"] = "changed"
	got.PageMemory["favorites"] = 99
	got.AppLocations["bundle:identity"] = "changed"
	if current.Aliases["favorite"] != "My App" || current.PageMemory["favorites"] != 2 || current.AppLocations["bundle:identity"] != "/Applications/Current.app" {
		t.Fatal("merge exposed input maps")
	}
}

func TestManagementMergeSetChangesDoNotResurrectConcurrentRemovals(t *testing.T) {
	base := defaultPreferences()
	base.Hidden = []string{"a", "b"}
	base.Favorites = []string{"a", "b"}
	current := clonePreferences(base)
	current.Hidden = []string{"b", "c"}
	current.Favorites = []string{"b", "c"}
	next := clonePreferences(base)
	next.Hidden = []string{"a", "b", "d"}
	next.Favorites = []string{"a", "b", "d"}
	got := MergeManagementPreferences(current, next, base)
	if !reflect.DeepEqual(got.Hidden, []string{"b", "c", "d"}) || !reflect.DeepEqual(got.Favorites, []string{"b", "c", "d"}) {
		t.Fatal("unchanged baseline membership was resurrected", got)
	}
}

func TestManagementMergeOrderingKeepsConcurrentNewEntries(t *testing.T) {
	base := defaultPreferences()
	base.Order = []string{"a", "b"}
	current := clonePreferences(base)
	current.Order = []string{"c", "a", "b"}
	next := clonePreferences(base)
	next.Order = []string{"b", "a"}
	got := MergeManagementPreferences(current, next, base)
	if !reflect.DeepEqual(got.Order, []string{"b", "a", "c"}) {
		t.Fatal(got.Order)
	}
	next.Order = nil
	got = MergeManagementPreferences(current, next, base)
	if !reflect.DeepEqual(got.Order, []string{"c"}) {
		t.Fatal("manual order reset erased an independent new order entry", got.Order)
	}
}

func TestManagementMergeCategoriesByIdentityAndField(t *testing.T) {
	base := defaultPreferences()
	base.Categories = append(base.Categories, Category{ID: "work", Name: "Work", Visible: true, AppIDs: []string{"a"}}, Category{ID: "deleted", Name: "Delete", Visible: true}, Category{ID: "removed-live", Name: "Removed", Visible: true})
	current := clonePreferences(base)
	current.Categories[slices.IndexFunc(current.Categories, func(c Category) bool { return c.ID == "work" })].Visible = false
	current.Categories = slices.DeleteFunc(current.Categories, func(c Category) bool { return c.ID == "removed-live" })
	current.Categories = append(current.Categories, Category{ID: "native-new", Name: "Native", Visible: true, AppIDs: []string{"b"}})
	next := clonePreferences(base)
	next.Categories = slices.DeleteFunc(next.Categories, func(c Category) bool { return c.ID == "deleted" })
	index := slices.IndexFunc(next.Categories, func(c Category) bool { return c.ID == "work" })
	next.Categories[index].Name = "Renamed"
	next.Categories[index].AppIDs = append(next.Categories[index].AppIDs, "c")
	next.Categories = append(next.Categories, Category{ID: "manager-new", Name: "Manager", Visible: true})
	got := MergeManagementPreferences(current, next, base)
	index = slices.IndexFunc(got.Categories, func(c Category) bool { return c.ID == "work" })
	if index < 0 || got.Categories[index].Name != "Renamed" || got.Categories[index].Visible || !reflect.DeepEqual(got.Categories[index].AppIDs, []string{"a", "c"}) {
		t.Fatal("independent category fields did not merge", got.Categories)
	}
	for _, id := range []string{"deleted", "removed-live"} {
		if slices.ContainsFunc(got.Categories, func(c Category) bool { return c.ID == id }) {
			t.Fatalf("deleted category %q resurrected", id)
		}
	}
	for _, id := range []string{"native-new", "manager-new"} {
		if !slices.ContainsFunc(got.Categories, func(c Category) bool { return c.ID == id }) {
			t.Fatalf("new category %q lost", id)
		}
	}
	if got.Categories[len(got.Categories)-1].ID != "native-new" {
		t.Fatal("concurrent new category was not retained after requested order", got.Categories)
	}
}

func TestManagementMergeAliasesApplyPerKeyAndRuntimeMapsStayLive(t *testing.T) {
	base := defaultPreferences()
	base.Aliases = map[string]string{"a": "Alpha", "removed": "Delete"}
	current := clonePreferences(base)
	current.Aliases["a"] = "Native rename"
	current.Aliases["new"] = "New alias"
	current.LastCategory = "favorites"
	current.PageMemory["favorites"] = 4
	current.AppLocations["bundle:current"] = "/Applications/Current.app"
	next := clonePreferences(base)
	delete(next.Aliases, "removed")
	next.Aliases["b"] = "Beta"
	next.PageMemory["favorites"] = 99
	next.AppLocations["bundle:stale"] = "/Applications/Stale.app"
	got := MergeManagementPreferences(current, next, base)
	if !reflect.DeepEqual(got.Aliases, map[string]string{"a": "Native rename", "new": "New alias", "b": "Beta"}) || !reflect.DeepEqual(got.PageMemory, current.PageMemory) || !reflect.DeepEqual(got.AppLocations, current.AppLocations) || got.LastCategory != "favorites" {
		t.Fatal(got)
	}
}

func TestManagementMergeDiskFailureDoesNotPublishMergedState(t *testing.T) {
	s := testService(t)
	base := s.Preferences()
	if _, err := s.State(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.configPath, 0700); err != nil {
		t.Fatal(err)
	}
	next := clonePreferences(base)
	next.Language = "en"
	if _, err := s.SaveManagementPreferencesWithBase(next, base); err == nil {
		t.Fatal("save over directory succeeded")
	}
	if s.Preferences().Language != base.Language {
		t.Fatal("failed merge was published")
	}
}
