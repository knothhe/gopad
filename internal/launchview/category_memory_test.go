package launchview

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/launcher"
)

type rememberingCatalog struct {
	mu           sync.Mutex
	state        launcher.State
	path         string
	writes       []string
	fail         bool
	firstStarted chan struct{}
	releaseFirst chan struct{}
}

func rememberingFixture(path string) *rememberingCatalog {
	state := fixture(5).state
	state.Preferences.LastCategory = "all"
	state.Preferences.Categories = launcher.DefaultCategories()
	return &rememberingCatalog{state: state, path: path}
}
func (f *rememberingCatalog) State() (launcher.State, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.state
	state.Apps = append([]launcher.App(nil), state.Apps...)
	state.Preferences.Categories = append([]launcher.Category(nil), state.Preferences.Categories...)
	return state, nil
}
func (f *rememberingCatalog) Refresh() (launcher.State, error) { return f.State() }
func (f *rememberingCatalog) Icon(string) ([]byte, error)      { return nil, fmt.Errorf("no fixture icon") }
func (f *rememberingCatalog) Launch(string) error              { return nil }
func (f *rememberingCatalog) SetLastCategory(id string) error {
	f.mu.Lock()
	f.writes = append(f.writes, id)
	first := len(f.writes) == 1
	started, release, fail := f.firstStarted, f.releaseFirst, f.fail
	f.mu.Unlock()
	if first && started != nil {
		close(started)
		<-release
	}
	if fail {
		return fmt.Errorf("disk unavailable")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.state.Preferences.LastCategory = id
	if f.path != "" {
		data, err := json.Marshal(f.state.Preferences)
		if err != nil {
			return err
		}
		return os.WriteFile(f.path, data, 0600)
	}
	return nil
}
func loadedRememberingPad(f *rememberingCatalog, callbacks Callbacks) *Launchpad {
	v := New(f, callbacks)
	v.state, _ = f.State()
	v.loaded = true
	return v
}
func waitCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("background state did not settle")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCategoryMemoryRestoresAndCloseFlushesNewestSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	f := rememberingFixture(path)
	f.firstStarted = make(chan struct{})
	f.releaseFirst = make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(f.releaseFirst) }) })
	v := loadedRememberingPad(f, Callbacks{})
	tt := ui.NewTester(v.View, 900, 650)
	tt.Key(0, ui.KeyTab)
	select {
	case <-f.firstStarted:
	case <-time.After(time.Second):
		t.Fatal("first category was not submitted")
	}
	tt.Key(0, ui.KeyTab)
	if v.category != "system" {
		t.Fatalf("second Tab chose %s", v.category)
	}
	v.ResetSearch()
	tt.Frame()
	if v.category != "system" {
		t.Fatal("hiding and reopening reset the category")
	}
	// Refresh returns a snapshot captured before the category save finishes.
	v.Reload()
	waitCondition(t, func() bool { v.mu.Lock(); defer v.mu.Unlock(); return v.iconEpoch > 0 })
	tt.Frame()
	if v.category != "system" {
		t.Fatal("an older refreshed snapshot reset the active category")
	}
	closed := make(chan struct{})
	go func() { v.Close(); close(closed) }()
	select {
	case <-closed:
		t.Fatal("Close returned before the pending save")
	default:
	}
	releaseOnce.Do(func() { close(f.releaseFirst) })
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close waited on UI work instead of finishing persistence")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved launcher.Preferences
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.LastCategory != "system" {
		t.Fatalf("last saved category %q", saved.LastCategory)
	}
	f.mu.Lock()
	writes := append([]string(nil), f.writes...)
	f.mu.Unlock()
	if len(writes) != 2 || writes[0] != "favorites" || writes[1] != "system" {
		t.Fatalf("save order %v", writes)
	}
	restarted := rememberingFixture(path)
	restarted.state.Preferences = saved
	restored := loadedRememberingPad(restarted, Callbacks{})
	defer restored.Close()
	ui.NewTester(restored.View, 900, 650)
	if restored.category != "system" {
		t.Fatalf("restart restored %s", restored.category)
	}
}

func TestRememberedCategoryFallsBackWhenHiddenOrRemoved(t *testing.T) {
	for _, saved := range []string{"system", "removed-custom"} {
		t.Run(saved, func(t *testing.T) {
			f := rememberingFixture("")
			f.state.Preferences.LastCategory = saved
			f.state.Preferences.Categories[2].Visible = false
			v := loadedRememberingPad(f, Callbacks{})
			ui.NewTester(v.View, 900, 650)
			v.Close()
			if v.category != "all" {
				t.Fatalf("invalid remembered category restored %s", v.category)
			}
			state, _ := f.State()
			if state.Preferences.LastCategory != "all" {
				t.Fatalf("fallback was not saved: %s", state.Preferences.LastCategory)
			}
		})
	}
}

func TestCategorySaveFailureIsVisibleAndCanRetry(t *testing.T) {
	f := rememberingFixture("")
	f.fail = true
	v := loadedRememberingPad(f, Callbacks{Locale: func() string { return "en" }})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	tt.Key(0, ui.KeyTab)
	waitCondition(t, v.categorySaveFailed)
	tt.Frame()
	if v.category != "favorites" || !tt.HasText("Category selection wasn't saved: disk unavailable") {
		t.Fatal("failed save lost the selection or its error")
	}
	f.mu.Lock()
	f.fail = false
	f.mu.Unlock()
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	waitCondition(t, func() bool { f.mu.Lock(); defer f.mu.Unlock(); return f.state.Preferences.LastCategory == "favorites" })
	waitCondition(t, func() bool { return !v.categorySaveFailed() })
	tt.Frame()
	if tt.HasText("Category selection wasn't saved: disk unavailable") {
		t.Fatal("successful retry kept an obsolete failure")
	}
}

func TestCategoryBarAlignsWithGridAndTranslatesWithoutReset(t *testing.T) {
	f := fixture(5)
	f.state.Preferences.Categories = append(launcher.DefaultCategories(), launcher.Category{ID: "work", Name: "创作", AppIDs: []string{"app-001"}, Visible: true})
	locale := "zh-CN"
	v := testPad(f, Callbacks{Locale: func() string { return locale }})
	defer v.Close()
	tt := ui.NewTester(v.View, 1440, 900)
	geometry := layout(1440, 900, 80)
	expectedX, _, _ := categoryGeometry(geometry, 80)
	strip, ok := tt.Find("应用分类")
	if !ok || math.Abs(float64(strip.X-expectedX)) > .1 || strip.Y+strip.H > geometry.Y {
		t.Fatalf("category strip does not align above grid: %+v geometry=%+v", strip, geometry)
	}
	if err := tt.Click("分类：创作"); err != nil {
		t.Fatal(err)
	}
	tt.Type("工具")
	locale = "en"
	tt.Frame()
	if v.category != "work" || v.query != "工具" {
		t.Fatal("language change reset the current context")
	}
	if _, ok := tt.Find("Search apps"); !ok {
		t.Fatal("native search label did not translate")
	}
	if _, ok := tt.Find("Category: Favorites"); !ok {
		t.Fatal("built-in category did not translate")
	}
	if _, ok := tt.Find("Category: 创作"); !ok {
		t.Fatal("custom category name was translated as UI copy")
	}
	for _, text := range tt.Texts() {
		if strings.Contains(text, "native.") {
			t.Fatalf("unresolved native copy key: %s", text)
		}
	}
}

func TestExplicitAllBeforeInitialLoadWinsOverOlderRememberedCategory(t *testing.T) {
	f := rememberingFixture("")
	f.state.Preferences.LastCategory = "favorites"
	v := New(f, Callbacks{})
	tt := ui.NewTester(v.View, 900, 650)
	if err := tt.Click("分类：全部"); err != nil {
		t.Fatal(err)
	}
	v.restoreCategory("favorites", true)
	v.Close()
	if v.category != "all" {
		t.Fatal("initial remembered snapshot overrode the explicit All selection")
	}
	state, _ := f.State()
	if state.Preferences.LastCategory != "all" {
		t.Fatal("explicit All selection was not persisted")
	}
}
