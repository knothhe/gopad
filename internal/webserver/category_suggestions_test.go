package webserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/knothhe/gopad/internal/launcher"
)

type suggestionsCatalog struct {
	mockCatalog
	state     launcher.State
	stateErr  error
	saves     int
	refreshes int
}

func (c *suggestionsCatalog) State() (launcher.State, error) { return c.state, c.stateErr }
func (c *suggestionsCatalog) SavePreferences(launcher.Preferences) (launcher.State, error) {
	c.saves++
	return c.State()
}
func (c *suggestionsCatalog) Refresh() (launcher.State, error) { c.refreshes++; return c.State() }

func TestCategorySuggestionsUseEffectiveLocaleAndPreviewDoesNotMutate(t *testing.T) {
	c := &suggestionsCatalog{state: launcher.State{Preferences: launcher.Preferences{Language: "system", Hidden: []string{"hidden"}}, Apps: []launcher.App{
		{ID: "a", Category: "public.app-category.developer-tools"},
		{ID: "b", Category: "public.app-category.developer-tools"},
		{ID: "hidden", Category: "public.app-category.developer-tools"},
		{ID: "system", Kind: "system", Category: "public.app-category.developer-tools"},
		{ID: "system-path", Path: "/System/Applications/Tool.app", Category: "public.app-category.developer-tools"},
		{ID: "named-only", Name: "Developer Tools", Category: "malformed"},
	}}}
	before, _ := json.Marshal(c.state)
	prepares := 0
	h := New(c, fstest.MapFS{}, fstest.MapFS{}, Controls{SystemLocale: func() string { return "zh-Hans-CN" }, PreparePreferences: func(launcher.Preferences, launcher.Preferences) (func(bool), error) {
		prepares++
		return func(bool) {}, nil
	}})
	w := request(h, "GET", "/api/category-suggestions", "", "")
	var got struct {
		Categories []launcher.Category `json:"categories"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || len(got.Categories) != 1 || got.Categories[0].Name != "开发" || !reflect.DeepEqual(got.Categories[0].AppIDs, []string{"a", "b"}) {
		t.Fatal(w.Code, w.Body.String())
	}
	after, _ := json.Marshal(c.state)
	if string(before) != string(after) || c.saves != 0 || c.refreshes != 0 || c.launches != 0 || prepares != 0 {
		t.Fatal("suggestion preview changed application or system state", c, prepares)
	}
	// An explicit application language takes precedence over the system locale.
	c.state.Preferences.Language = "en"
	w = request(h, "GET", "/api/category-suggestions", "", "")
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Categories) != 1 || got.Categories[0].Name != "Development" {
		t.Fatal(w.Body.String())
	}
}

func TestCategorySuggestionsRejectMutationMethodsAndHandleCatalogFailure(t *testing.T) {
	c := &suggestionsCatalog{}
	h := New(c, fstest.MapFS{}, fstest.MapFS{}, Controls{})
	for _, method := range []string{"POST", "PUT", "DELETE", "HEAD"} {
		w := request(h, method, "/api/category-suggestions", "{malformed", "")
		if w.Code != http.StatusMethodNotAllowed || w.Header().Get("Allow") != "GET" {
			t.Fatal(method, w.Code, w.Body.String())
		}
	}
	if c.saves != 0 || c.refreshes != 0 {
		t.Fatal("unsupported method mutated state")
	}
	c.stateErr = errors.New("catalog unavailable")
	w := request(h, "GET", "/api/category-suggestions", "", "")
	if w.Code != http.StatusInternalServerError {
		t.Fatal(w.Code, w.Body.String())
	}
	c.stateErr = nil
	c.state.Apps = []launcher.App{{ID: "one", Category: "public.app-category.developer-tools"}, {ID: "invalid", Name: "Developer", Category: "not-a-category"}}
	w = request(h, "GET", "/api/category-suggestions", "", "")
	var got struct {
		Categories []launcher.Category `json:"categories"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || len(got.Categories) != 0 {
		t.Fatal("invalid metadata generated a guessed category", w.Code, w.Body.String())
	}
}

type baselinePreferencesCatalog struct{ preferencesCatalog }

func (c *baselinePreferencesCatalog) SaveManagementPreferencesWithBase(next, base launcher.Preferences) (launcher.State, error) {
	return c.SavePreferences(launcher.MergeManagementPreferences(c.preferences, next, base))
}

func TestBaselinePreferencesPrepareUsesMergedCurrentSystemChoices(t *testing.T) {
	base := launcher.Preferences{Shortcut: "Ctrl+Alt+Space", LaunchAtLogin: false, Language: "en", IconSize: 80}
	next := base
	next.IconSize = 96
	current := base
	current.Shortcut = "Ctrl+Alt+G"
	current.LaunchAtLogin = true
	current.Recent = []string{"newly-used"}
	c := &baselinePreferencesCatalog{preferencesCatalog: preferencesCatalog{preferences: current}}
	prepares, finishes := 0, 0
	h := New(c, fstest.MapFS{}, fstest.MapFS{}, Controls{PreparePreferences: func(previous, candidate launcher.Preferences) (func(bool), error) {
		prepares++
		if previous.Shortcut != "Ctrl+Alt+G" || candidate.Shortcut != "Ctrl+Alt+G" || !candidate.LaunchAtLogin || candidate.IconSize != 96 {
			t.Fatal("native preparation saw unchanged stale system preferences", previous, candidate)
		}
		return func(saved bool) {
			if !saved {
				t.Error("merged save was not committed")
			}
			finishes++
		}, nil
	}})
	body, _ := json.Marshal(struct {
		launcher.Preferences
		BasePreferences launcher.Preferences `json:"basePreferences"`
	}{next, base})
	w := request(h, "PUT", "/api/preferences", string(body), "")
	if w.Code != http.StatusOK || prepares != 1 || finishes != 1 || c.saves != 1 || c.preferences.Shortcut != "Ctrl+Alt+G" || !c.preferences.LaunchAtLogin || c.preferences.IconSize != 96 || !reflect.DeepEqual(c.preferences.Recent, []string{"newly-used"}) {
		t.Fatal(w.Code, w.Body.String(), prepares, finishes, c)
	}
	for _, malformed := range []string{`{"basePreferences":"invalid"}`, `{"basePreferences":{"unknownField":true}}`, `{"basePreferences":{}} {}`} {
		w = request(h, "PUT", "/api/preferences", malformed, "")
		if w.Code != http.StatusBadRequest {
			t.Fatal("malformed baseline accepted", malformed, w.Code)
		}
	}
	if prepares != 1 || c.saves != 1 {
		t.Fatal("malformed baseline reached prepare or persistence", prepares, c.saves)
	}
}
