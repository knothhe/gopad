package launchview

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/launcher"
)

func TestGlobalSearchRanksNamesAndPreservesBrowseContext(t *testing.T) {
	f := fixture(45)
	f.state.Preferences.SearchScope = "all"
	f.state.Preferences.SearchRanking = "relevance"
	f.state.Preferences.Favorites = []string{"app-001"}
	f.state.Preferences.Categories = launcher.DefaultCategories()
	f.state.Apps[0].Name = "Terminal Tools"
	f.state.Apps[4].Name = "Terminal"
	f.state.Preferences.Order = []string{"app-000"}
	v := testPad(f, Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	v.selectCategory("favorites")
	tt.Frame()
	tt.Type("terminal")
	if len(v.filtered) != 2 || v.filtered[0].ID != "app-004" || v.selected != 0 || v.category != "favorites" {
		t.Fatalf("global ranked results lost context: category=%s selected=%d results=%v", v.category, v.selected, v.filtered)
	}
	if !tt.HasText(v.t("native.searchScope.all")) {
		t.Fatal("global search scope is not visible")
	}
	v.clearSearch()
	tt.Frame()
	if v.category != "favorites" || len(v.filtered) != 1 {
		t.Fatal("clearing search did not restore favorites")
	}
	v.state.Preferences.SearchScope = "current"
	tt.Type("terminal")
	if len(v.filtered) != 0 {
		t.Fatal("current-category search escaped favorites")
	}
	if !tt.HasText(v.t("native.searchScope.current")) {
		t.Fatal("current search scope is not visible")
	}
}

func TestSearchAndEscapeRestorePageThenDismiss(t *testing.T) {
	v := testPad(fixture(80), Callbacks{})
	defer v.Close()
	var hidden atomic.Int32
	v.callbacks.Hide = func() { hidden.Add(1) }
	v.state.Preferences.EscapeClearsSearch = true
	tt := ui.NewTester(v.View, 900, 650)
	tt.Key(0, ui.KeyPageDown)
	tt.Key(0, ui.KeyPageDown)
	tt.Advance(contentSlide.Duration)
	if v.page != 2 {
		t.Fatal("fixture failed to page")
	}
	tt.Type("工具 001")
	if v.page != 0 || len(v.filtered) != 1 {
		t.Fatal("query did not start on first result page")
	}
	tt.Key(0, ui.KeyEscape)
	if v.query != "" || v.page != 2 || hidden.Load() != 0 {
		t.Fatalf("first Escape lost browse page: query=%s page=%d hidden=%d", v.query, v.page, hidden.Load())
	}
	tt.Key(0, ui.KeyEscape)
	if hidden.Load() != 1 {
		t.Fatal("second Escape did not dismiss")
	}
	v.state.Preferences.EscapeClearsSearch = false
	tt.Type("工具")
	tt.Key(0, ui.KeyEscape)
	if hidden.Load() != 2 || v.query == "" {
		t.Fatal("immediate Escape preference was ignored")
	}
}

func TestNativePagingStopsAtBoundariesAndCanLoop(t *testing.T) {
	v, now := swipePad()
	v.state.Preferences.LoopPages = false
	v.changePage(-1)
	if v.page != 0 {
		t.Fatal("previous wrapped despite setting")
	}
	v.handlePageScrollPhase(ui.InputEvent{Kind: ui.InputScroll, DX: -300, Precise: true}, now, scrollBegan)
	if len(v.slides.layers) != 1 || abs(v.slides.from) > .1 {
		t.Fatal("edge swipe duplicated page or did not resist")
	}
	v.handlePageScrollPhase(ui.InputEvent{Kind: ui.InputScroll, Precise: true}, now.Add(30*time.Millisecond), scrollEnded)
	if v.page != 0 {
		t.Fatal("edge swipe wrapped")
	}
	closeOffset(t, v.slides.position(now.Add(time.Second)), 0)
	v.changePage(100)
	if v.page != 2 {
		t.Fatal("next did not stop at last page")
	}
	v.state.Preferences.LoopPages = true
	v.changePage(3)
	if v.page != 0 || v.pageDirection != 1 {
		t.Fatal("loop did not preserve forward animation")
	}
}

type pageCatalog struct {
	*fixtureCatalog
	mu      sync.Mutex
	written map[string]int
	fail    bool
}

func (f *pageCatalog) SetPage(category string, page int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return fmt.Errorf("disk unavailable")
	}
	if f.written == nil {
		f.written = make(map[string]int)
	}
	f.written[category] = page
	return nil
}

func TestPageMemoryRestoresPerCategoryAndFlushesOnClose(t *testing.T) {
	f := &pageCatalog{fixtureCatalog: fixture(80)}
	f.state.Preferences.RememberPage = true
	f.state.Preferences.Categories = launcher.DefaultCategories()
	f.state.Preferences.PageMemory = map[string]int{"all": 2}
	v := New(f, Callbacks{})
	v.state, v.loaded = f.state, true
	tt := ui.NewTester(v.View, 900, 650)
	if v.page != 2 {
		t.Fatalf("saved page not restored: %d", v.page)
	}
	v.selectCategory("favorites")
	tt.Frame()
	if v.page != 0 {
		t.Fatal("favorites inherited all page")
	}
	v.selectCategory("all")
	tt.Frame()
	if v.page != 2 {
		t.Fatal("switching back lost all page")
	}
	tt.Key(0, ui.KeyPageDown)
	v.ResetSearch()
	tt.Frame()
	if v.page != 3 {
		t.Fatal("reopening lost current page")
	}
	v.Close()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.written["all"] != 3 {
		t.Fatalf("Close failed to flush latest page: %v", f.written)
	}
}

func TestPageSaveFailureRetainsIntentAndCanRetry(t *testing.T) {
	f := &pageCatalog{fixtureCatalog: fixture(50), fail: true}
	f.state.Preferences.RememberPage = true
	v := New(f, Callbacks{})
	defer v.Close()
	v.state, v.loaded = f.state, true
	tt := ui.NewTester(v.View, 900, 650)
	tt.Key(0, ui.KeyPageDown)
	waitCondition(t, v.pageSaveFailed)
	tt.Frame()
	if !tt.HasText(v.t("native.error.retry")) {
		t.Fatal("failed page save has no retry")
	}
	f.mu.Lock()
	f.fail = false
	f.mu.Unlock()
	if err := tt.Click(v.t("native.error.retry")); err != nil {
		t.Fatal(err)
	}
	waitCondition(t, func() bool { return !v.pageSaveFailed() })
	v.Close()
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.written["all"] != 1 {
		t.Fatalf("retry lost current page: %v", f.written)
	}
}

func TestStandardTabSettingKeepsCategorySwitchShortcut(t *testing.T) {
	v := testPad(fixture(5), Callbacks{})
	defer v.Close()
	v.state.Preferences.TabCyclesCategories = false
	tt := ui.NewTester(v.View, 900, 650)
	tt.Key(0, ui.KeyTab)
	if v.category != "all" {
		t.Fatal("Tab changed category with standard-focus setting")
	}
	tt.Key(ui.Ctrl, ui.KeyTab)
	if v.category != "favorites" {
		t.Fatal("Ctrl+Tab no longer switches categories")
	}
}

func TestRecentOrderBreaksSearchTiesWithoutOverridingExactNames(t *testing.T) {
	f := fixture(6)
	f.state.Preferences.ShowRecentApps = true
	f.state.Preferences.Recent = []string{"app-003", "app-002", "app-001"}
	f.state.Preferences.Order = []string{"app-001", "app-002"}
	f.state.Preferences.Hidden = []string{"app-002"}
	apps := appsForCategory(f.state, "", launcher.Category{ID: "recent"})
	if len(apps) != 2 || apps[0].ID != "app-003" || apps[1].ID != "app-001" {
		t.Fatalf("recent browse order: %v", apps)
	}
	f.state.Apps[4].Name = "工具"
	apps = appsFor(f.state, "工具")
	if apps[0].ID != "app-004" || apps[1].ID != "app-003" {
		t.Fatalf("recent order replaced exact name relevance: %v", apps)
	}
}

func TestHideUndoShortcutDoesNotConsumeSearchTextUndo(t *testing.T) {
	v := testPad(fixture(5), Callbacks{})
	defer v.Close()
	v.actions.undoID = "app-000"
	tt := ui.NewTester(v.View, 900, 650)
	tt.Type("工具")
	before := v.query
	tt.Key(ui.Cmd, ui.KeyZ)
	if v.query == before {
		t.Fatal("hide undo shortcut consumed the search field's undo")
	}
	if !v.hasHiddenUndo() {
		t.Fatal("text undo removed app hide undo history")
	}
}
