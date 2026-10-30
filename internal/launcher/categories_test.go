package launcher

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func categoryIDs(categories []Category) []string {
	ids := make([]string, len(categories))
	for i, category := range categories {
		ids[i] = category.ID
	}
	return ids
}

func TestLegacyPreferencesMigrateToDefaultCategories(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	legacy := `{"hidden":["removed-app"],"order":["b","a"],"iconSize":96,"favorites":["a"]}`
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	p := s.preferences
	if !reflect.DeepEqual(categoryIDs(p.Categories), []string{"all", "favorites", "system", "hidden"}) {
		t.Fatal("default category order", p.Categories)
	}
	if !reflect.DeepEqual(categoryIDs(VisibleCategories(p)), []string{"all", "favorites", "system"}) {
		t.Fatal("default visible categories", p.Categories)
	}
	if p.Categories[0].Name != "全部" || p.Categories[1].Name != "收藏" || p.Categories[2].Name != "系统" || p.Categories[3].Name != "隐藏" {
		t.Fatal("builtin names", p.Categories)
	}
	if p.IconSize != 96 || !reflect.DeepEqual(p.Order, []string{"b", "a"}) || !reflect.DeepEqual(p.Hidden, []string{"removed-app"}) || !reflect.DeepEqual(p.Favorites, []string{"a"}) {
		t.Fatal("migration changed existing preferences", p)
	}
	if !reflect.DeepEqual(CategoriesFor(Preferences{}), p.Categories) {
		t.Fatal("uninitialized preferences have different defaults")
	}
}

func TestCategoryOrderVisibilityAndMissingBuiltins(t *testing.T) {
	p := Preferences{Categories: []Category{
		{ID: "work", Name: "工作", AppIDs: []string{"b", "uninstalled"}, Visible: true},
		{ID: "system", Name: "tampered name", AppIDs: []string{"a"}, Visible: false},
		{ID: "all", Name: "renamed", Visible: false},
	}}
	categories := CategoriesFor(p)
	if !reflect.DeepEqual(categoryIDs(categories), []string{"work", "system", "all", "favorites", "hidden"}) {
		t.Fatal("custom/builtin order was not preserved", categories)
	}
	if !reflect.DeepEqual(categoryIDs(VisibleCategories(p)), []string{"work", "favorites"}) {
		t.Fatal("visibility was not preserved", VisibleCategories(p))
	}
	if categories[0].Name != "工作" || !reflect.DeepEqual(categories[0].AppIDs, []string{"b", "uninstalled"}) {
		t.Fatal("custom category changed", categories[0])
	}
	if categories[1].Name != "系统" || categories[2].Name != "全部" || len(categories[1].AppIDs) != 0 {
		t.Fatal("builtin metadata not normalized", categories)
	}
	for i := range categories {
		categories[i].Visible = false
	}
	visible := VisibleCategories(Preferences{Categories: categories})
	if len(visible) != 1 || visible[0].ID != "all" {
		t.Fatal("all-hidden preferences need the all category fallback", visible)
	}
}

func TestCategoryMembersPersistAndCopiesAreIndependent(t *testing.T) {
	s := testService(t)
	p := Preferences{Categories: []Category{{ID: "work", Name: "工作", AppIDs: []string{"b", "a", "b", "uninstalled"}, Visible: true}}}
	state, err := s.SavePreferences(p)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(state.Preferences.Categories[0].AppIDs, []string{"b", "a", "uninstalled"}) {
		t.Fatal("membership lost order, deduplication, or missing apps", state.Preferences.Categories)
	}
	p.Categories[0].AppIDs[0] = "mutated-input"
	state.Preferences.Categories[0].Name = "mutated-output"
	state.Preferences.Categories[0].AppIDs[0] = "mutated-output"
	state, _ = s.State()
	if state.Preferences.Categories[0].Name != "工作" || state.Preferences.Categories[0].AppIDs[0] != "b" {
		t.Fatal("saved preferences share mutable storage")
	}
	categories := CategoriesFor(state.Preferences)
	categories[0].AppIDs[0] = "mutated-helper"
	visible := VisibleCategories(state.Preferences)
	visible[0].AppIDs[0] = "mutated-visible"
	if state.Preferences.Categories[0].AppIDs[0] != "b" {
		t.Fatal("helper returned shared member storage")
	}
	reopened, err := New(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened.preferences.Categories, state.Preferences.Categories) {
		t.Fatal("custom categories lost after reopen", reopened.preferences.Categories)
	}
	// Category membership does not reorder the stable catalog; the view applies
	// the user's explicit global app order when needed.
	if !reflect.DeepEqual([]string{state.Apps[0].ID, state.Apps[1].ID}, []string{"a", "b"}) {
		t.Fatal("category membership reordered the catalog", state.Apps)
	}
	state.Preferences.Categories = []Category{{ID: "hidden", Name: "ignored", Visible: false}}
	state, err = s.SavePreferences(state.Preferences)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(categoryIDs(state.Preferences.Categories), []string{"hidden", "all", "favorites", "system"}) {
		t.Fatal("deleting a custom category removed builtins", state.Preferences.Categories)
	}
}

func TestCategoryContainsRespectsHiddenFavoritesSystemAndCustomMembership(t *testing.T) {
	p := Preferences{Hidden: []string{"hidden-app"}, Favorites: []string{"favorite", "hidden-app"}}
	tests := []struct {
		name     string
		category Category
		app      App
		want     bool
	}{
		{"all", Category{ID: "all"}, App{ID: "normal"}, true},
		{"all-excludes-hidden", Category{ID: "all"}, App{ID: "hidden-app"}, false},
		{"hidden-exclusive", Category{ID: "hidden"}, App{ID: "hidden-app"}, true},
		{"hidden-excludes-visible", Category{ID: "hidden"}, App{ID: "normal"}, false},
		{"favorite", Category{ID: "favorites"}, App{ID: "favorite"}, true},
		{"favorite-excludes-hidden", Category{ID: "favorites"}, App{ID: "hidden-app"}, false},
		{"system-kind", Category{ID: "system"}, App{ID: "system", Kind: "system"}, true},
		{"system-path", Category{ID: "system"}, App{ID: "system", Path: "/System/Applications/Notes.app"}, true},
		{"system-path-boundary", Category{ID: "system"}, App{ID: "normal", Path: "/Systematic/Notes.app"}, false},
		{"custom-member", Category{ID: "work", AppIDs: []string{"normal"}, Visible: false}, App{ID: "normal"}, true},
		{"custom-excludes-hidden", Category{ID: "work", AppIDs: []string{"hidden-app"}}, App{ID: "hidden-app"}, false},
		{"custom-nonmember", Category{ID: "work", AppIDs: []string{"other"}}, App{ID: "normal"}, false},
		{"empty-app", Category{ID: "all"}, App{}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CategoryContains(test.category, test.app, p); got != test.want {
				t.Fatalf("got %v want %v", got, test.want)
			}
		})
	}
}

func TestInvalidCategoriesAndMemberLimitsAreNormalized(t *testing.T) {
	categories := []Category{
		{ID: "../invalid", Name: "ignored", Visible: true},
		{ID: strings.Repeat("a", 81), Name: "ignored", Visible: true},
		{ID: "blank", Name: " \n\t ", Visible: true},
		{ID: " work ", Name: "  工\n作\x00  ", AppIDs: []string{"a", " a ", "", "../bad", "bad\x00", strings.Repeat("x", 129), "not-installed"}, Visible: true},
		{ID: "work", Name: "duplicate must lose", AppIDs: []string{"other"}, Visible: false},
		{ID: "long-name", Name: strings.Repeat("长", 45), Visible: false},
		{ID: "all", Name: "tampered", Visible: false},
		{ID: "all", Name: "duplicate", Visible: true},
	}
	got := CategoriesFor(Preferences{Categories: categories})
	if !reflect.DeepEqual(categoryIDs(got), []string{"work", "long-name", "all", "favorites", "system", "hidden"}) {
		t.Fatal("invalid or duplicate categories survived", got)
	}
	if got[0].Name != "工作" || !got[0].Visible || !reflect.DeepEqual(got[0].AppIDs, []string{"a", "not-installed"}) {
		t.Fatal("custom normalization", got[0])
	}
	if len([]rune(got[1].Name)) != 40 || got[2].Visible {
		t.Fatal("name length or first duplicate visibility lost", got)
	}
	var many []Category
	for i := 0; i < 70; i++ {
		many = append(many, Category{ID: fmt.Sprintf("custom-%d", i), Name: "分区", Visible: true})
	}
	got = CategoriesFor(Preferences{Categories: many})
	if len(got) != maxCustomCategories+4 {
		t.Fatal("custom category limit or required builtins lost", len(got))
	}
	var ids []string
	for i := 0; i < maxAppIDs+5; i++ {
		ids = append(ids, fmt.Sprintf("app-%d", i))
	}
	got = CategoriesFor(Preferences{Categories: []Category{{ID: "limited", Name: "分区", AppIDs: ids, Visible: true}}})
	if len(got[0].AppIDs) != maxAppIDs {
		t.Fatal("member limit not applied", len(got[0].AppIDs))
	}
}
