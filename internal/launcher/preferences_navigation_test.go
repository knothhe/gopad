package launcher

import (
	"path/filepath"
	"testing"
)

func TestRememberedCategorySurvivesStaleManagementSaveAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	s.scan = func() ([]App, error) { return []App{}, nil }
	stale := s.Preferences()
	if err := s.SetLastCategory("favorites"); err != nil {
		t.Fatal(err)
	}
	stale.Language = "en"
	stale.IconSize = 96
	state, err := s.SaveManagementPreferences(stale)
	if err != nil {
		t.Fatal(err)
	}
	if state.Preferences.LastCategory != "favorites" || state.Preferences.Language != "en" {
		t.Fatal(state.Preferences)
	}
	if err := s.SetLastCategory("system"); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Preferences()
	if got.LastCategory != "system" || got.Language != "en" || got.IconSize != 96 {
		t.Fatal(got)
	}
}

func TestRememberedCategoryFallsBackAndUnknownLanguageIsRetained(t *testing.T) {
	p := defaultPreferences()
	p.LastCategory = "favorites"
	p.Language = "future-language"
	for i := range p.Categories {
		if p.Categories[i].ID == "favorites" {
			p.Categories[i].Visible = false
		}
	}
	got := normalizePreferences(p)
	if got.LastCategory != "all" || got.Language != "future-language" {
		t.Fatal(got)
	}
	got = normalizePreferences(Preferences{})
	if got.LastCategory != "all" || got.Language != "system" {
		t.Fatal(got)
	}
}
