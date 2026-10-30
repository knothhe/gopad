package launchview

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/launcher"
)

type fixtureCatalog struct {
	state    launcher.State
	launched chan string
}

func (f *fixtureCatalog) State() (launcher.State, error)   { return f.state, nil }
func (f *fixtureCatalog) Refresh() (launcher.State, error) { return f.state, nil }
func (f *fixtureCatalog) Icon(string) ([]byte, error)      { return nil, fmt.Errorf("no fixture icon") }
func (f *fixtureCatalog) Launch(id string) error           { f.launched <- id; return nil }
func fixture(count int) *fixtureCatalog {
	f := &fixtureCatalog{launched: make(chan string, 4), state: launcher.State{Preferences: launcher.Preferences{IconSize: 80}}}
	for i := 0; i < count; i++ {
		f.state.Apps = append(f.state.Apps, launcher.App{ID: fmt.Sprintf("app-%03d", i), Name: fmt.Sprintf("工具 %03d", i), Path: fmt.Sprintf("/Applications/Tool%d.app", i), BundleID: fmt.Sprintf("dev.gopad.tool%d", i)})
	}
	return f
}
func testPad(f *fixtureCatalog, cb Callbacks) *Launchpad {
	v := New(f, cb)
	v.state = f.state
	v.loaded = true
	return v
}

func TestHiddenOrderAndSearch(t *testing.T) {
	f := fixture(5)
	f.state.Preferences.Hidden = []string{"app-001"}
	f.state.Preferences.Order = []string{"app-004", "app-001", "app-002"}
	apps := appsFor(f.state, "")
	var ids []string
	for _, a := range apps {
		ids = append(ids, a.ID)
	}
	if want := []string{"app-004", "app-002", "app-000", "app-003"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("visible order %v, want %v", ids, want)
	}
	if got := appsFor(f.state, " TOOL4 "); len(got) != 1 || got[0].ID != "app-004" {
		t.Fatalf("bundle search results %v", got)
	}
	if got := appsFor(f.state, "工具 001"); len(got) != 0 {
		t.Fatalf("hidden app appeared in search: %v", got)
	}
}

func TestPinyinSearchRespectsCategoriesAndHiddenApps(t *testing.T) {
	f := fixture(3)
	f.state.Apps[0].Name, f.state.Apps[1].Name, f.state.Apps[2].Name = "终端", "重庆音乐", "微信"
	f.state.Preferences.Hidden = []string{"app-002"}
	f.state.Preferences.Favorites = []string{"app-001"}
	for _, query := range []string{"zhongduan", " ZD ", "ＺＤ", "zhong端"} {
		if got := appsFor(f.state, query); len(got) != 1 || got[0].ID != "app-000" {
			t.Fatalf("query %q results %v", query, got)
		}
	}
	if got := appsForCategory(f.state, "cqyy", launcher.Category{ID: "favorites"}); len(got) != 1 || got[0].ID != "app-001" {
		t.Fatalf("favorite pinyin results %v", got)
	}
	if got := appsForCategory(f.state, "zd", launcher.Category{ID: "favorites"}); len(got) != 0 {
		t.Fatalf("pinyin search escaped its category: %v", got)
	}
	if got := appsFor(f.state, "wx"); len(got) != 0 {
		t.Fatalf("hidden app appeared in pinyin search: %v", got)
	}
	if got := appsForCategory(f.state, "wx", launcher.Category{ID: "hidden"}); len(got) != 1 || got[0].ID != "app-002" {
		t.Fatalf("hidden category pinyin results %v", got)
	}
}

func TestNativeSearchKeyboardAndGlassControls(t *testing.T) {
	f := fixture(8)
	f.state.Apps[3].Name = "终端"
	var hidden atomic.Int32
	managed := 0
	v := testPad(f, Callbacks{Hide: func() { hidden.Add(1) }, OpenManager: func() { managed++ }})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	if _, ok := tt.Find("管理应用"); !ok {
		t.Fatal("missing native management gear")
	}
	if _, ok := tt.Find("收起启动台"); !ok {
		t.Fatal("missing native close control")
	}
	for _, label := range []string{"所有应用", "个人收藏", "整理应用", "偏好设置"} {
		if tt.HasText(label) {
			t.Fatalf("management UI leaked into launchpad: %s", label)
		}
	}
	tt.Type("终端")
	if len(v.filtered) != 1 || v.filtered[0].Name != "终端" {
		t.Fatalf("search results %v", v.filtered)
	}
	tt.Key(0, ui.KeyEnter)
	select {
	case id := <-f.launched:
		if id != "app-003" {
			t.Fatalf("opened %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("Enter did not launch search result")
	}

	tt.Key(ui.Cmd, ui.KeyComma)
	if managed != 1 {
		t.Fatalf("management shortcut count %d", managed)
	}
	if image := tt.Image(); image.Bounds().Dx() != 900 || image.Bounds().Dy() != 650 {
		t.Fatalf("glass view rendered wrong bounds: %v", image.Bounds())
	}
}

func TestGridArrowNavigationAndEnter(t *testing.T) {
	f := fixture(30)
	v := testPad(f, Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	tt.Key(0, ui.KeyDown)
	if v.selected != 0 {
		t.Fatalf("Down from search selected %d, want first app", v.selected)
	}
	tt.Key(0, ui.KeyRight)
	if v.selected != 1 {
		t.Fatalf("Right selected %d, want 1", v.selected)
	}
	tt.Key(0, ui.KeyDown)
	want := 1 + v.columns
	if v.selected != want {
		t.Fatalf("Down selected %d, want %d", v.selected, want)
	}
	tt.Key(0, ui.KeyEnter)
	select {
	case id := <-f.launched:
		if id != fmt.Sprintf("app-%03d", want) {
			t.Fatalf("Enter opened %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("Enter did not launch focused grid app")
	}
}

func TestEscapeAndCloseHideEvenWhileSearching(t *testing.T) {
	hidden := 0
	v := testPad(fixture(8), Callbacks{Hide: func() { hidden++ }})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	tt.Type("工具")
	tt.Key(0, ui.KeyEscape)
	if hidden != 1 || v.query != "工具" {
		t.Fatalf("Escape should hide directly: hidden=%d query=%q", hidden, v.query)
	}
	if err := tt.Click("收起启动台"); err != nil {
		t.Fatal(err)
	}
	if hidden != 2 {
		t.Fatalf("close did not hide: %d", hidden)
	}
}

func TestPagingMotionAndSearchDuringTransition(t *testing.T) {
	v := testPad(fixture(35), Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	first, _ := tt.Find("工具 000")
	label := fmt.Sprintf("工具 %03d", v.pageSize)
	tt.Key(0, ui.KeyPageDown)
	tt.Advance(80 * time.Millisecond)
	moving, ok := tt.Find(label)
	if !ok || moving.X <= first.X {
		t.Fatalf("next page did not slide in: %v found=%v", moving, ok)
	}
	tt.Key(0, ui.KeyDown)
	if v.focus != fmt.Sprintf("app-%03d", v.pageSize) {
		t.Fatalf("ArrowDown focused the outgoing page during animation: %s", v.focus)
	}
	tt.Key(ui.Cmd, ui.KeyF)
	tt.Advance(180 * time.Millisecond)
	settled, ok := tt.Find(label)
	if !ok || math.Abs(float64(settled.X-first.X)) > 1 {
		t.Fatalf("next page did not settle: %v found=%v", settled, ok)
	}
	tt.Key(0, ui.KeyPageDown)
	tt.Advance(40 * time.Millisecond)
	tt.Type("工具 001")
	if v.page != 0 || len(v.filtered) != 1 {
		t.Fatal("search during paging retained the previous page")
	}
	if result, ok := tt.Find("工具 001"); !ok || math.Abs(float64(result.X-first.X)) > 1 {
		t.Fatalf("search result should appear immediately: %v found=%v", result, ok)
	}
	v.query = ""
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	tt.Key(0, ui.KeyPageDown)
	if result, ok := tt.Find(label); !ok || math.Abs(float64(result.X-first.X)) > 1 {
		t.Fatalf("Reduce Motion should change pages immediately: %v found=%v", result, ok)
	}
}

func TestCategoryTabCyclesBothDirectionsAndSkipsHidden(t *testing.T) {
	f := fixture(12)
	f.state.Preferences.Categories = append(launcher.DefaultCategories(), launcher.Category{ID: "work", Name: "工作", AppIDs: []string{"app-002"}, Visible: true})
	f.state.Preferences.Favorites = []string{"app-000", "app-001"}
	f.state.Apps[3].Kind = "system"
	v := testPad(f, Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	if !tt.HasText("Tab 切换 · ⇧Tab 返回") {
		t.Fatal("missing visible category shortcut hint")
	}
	if _, ok := tt.Find("分类：隐藏"); ok {
		t.Fatal("hidden category appeared by default")
	}
	tt.Type("工具")
	tt.Key(0, ui.KeyTab)
	if v.category != "favorites" || v.query != "" || v.page != 0 || v.selected != -1 {
		t.Fatalf("category switch did not reset search: category=%s query=%q page=%d selected=%d", v.category, v.query, v.page, v.selected)
	}
	for _, want := range []string{"system", "work", "all"} {
		tt.Key(0, ui.KeyTab)
		if v.category != want {
			t.Fatalf("Tab category %s, want %s", v.category, want)
		}
	}
	for _, want := range []string{"work", "system", "favorites", "all"} {
		tt.Key(ui.Shift, ui.KeyTab)
		if v.category != want {
			t.Fatalf("ShiftTab category %s, want %s", v.category, want)
		}
	}
	if err := tt.Click("分类：收藏"); err != nil {
		t.Fatal(err)
	}
	if v.category != "favorites" || len(v.filtered) != 2 {
		t.Fatal("clickable category did not filter favorites")
	}
}

func TestCategoryTabFromGridPreservesArrowAndEnter(t *testing.T) {
	f := fixture(20)
	f.state.Preferences.Favorites = []string{"app-003", "app-007"}
	v := testPad(f, Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	tt.Key(0, ui.KeyDown)
	if v.focus != "app-000" {
		t.Fatal("grid did not get focus")
	}
	tt.Key(0, ui.KeyTab)
	if v.category != "favorites" || v.focus != "search" {
		t.Fatalf("Tab from grid kept category=%s focus=%s", v.category, v.focus)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyRight)
	tt.Key(0, ui.KeyEnter)
	select {
	case id := <-f.launched:
		if id != "app-007" {
			t.Fatalf("filtered grid opened %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("filtered grid Enter did not launch")
	}
}

func TestCustomCategoryAndEnabledHiddenCategoryFiltering(t *testing.T) {
	f := fixture(6)
	f.state.Preferences.Hidden = []string{"app-002"}
	custom := launcher.Category{ID: "work", Name: "工作", AppIDs: []string{"app-001", "app-002", "app-005"}, Visible: true}
	apps := appsForCategory(f.state, "", custom)
	if len(apps) != 2 || apps[0].ID != "app-001" || apps[1].ID != "app-005" {
		t.Fatalf("custom category results %v", apps)
	}
	f.state.Preferences.Categories = launcher.DefaultCategories()
	f.state.Preferences.Categories[3].Visible = true
	v := testPad(f, Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	if err := tt.Click("分类：隐藏"); err != nil {
		t.Fatal(err)
	}
	if len(v.filtered) != 1 || v.filtered[0].ID != "app-002" {
		t.Fatalf("enabled hidden category results %v", v.filtered)
	}
}

func TestCategoryFallbackAfterReloadAndReopen(t *testing.T) {
	f := fixture(3)
	f.state.Preferences.Categories = append(launcher.DefaultCategories(), launcher.Category{ID: "work", Name: "工作", AppIDs: []string{"app-000"}, Visible: true})
	updates := make(chan struct{}, 64)
	v := testPad(f, Callbacks{Invalidate: func() {
		select {
		case updates <- struct{}{}:
		default:
		}
	}})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	if err := tt.Click("分类：工作"); err != nil {
		t.Fatal(err)
	}
	categories := append([]launcher.Category(nil), f.state.Preferences.Categories...)
	categories[len(categories)-1].Visible = false
	f.state.Preferences.Categories = categories
	v.Reload()
	deadline := time.After(time.Second)
	for {
		state, _, _ := v.snapshot()
		found := false
		for _, category := range launcher.VisibleCategories(state.Preferences) {
			if category.ID == "work" {
				found = true
			}
		}
		if !found {
			break
		}
		select {
		case <-updates:
		case <-deadline:
			t.Fatal("reload did not publish updated categories")
		}
	}
	tt.Frame()
	if v.category != "all" {
		t.Fatalf("invalid category did not fall back: %s", v.category)
	}
	v.selectCategory("favorites")
	v.ResetSearch()
	tt.Frame()
	if v.category != "favorites" {
		t.Fatalf("reopen forgot the current category: %s", v.category)
	}
	v.mu.Lock()
	v.state.Preferences.Categories = launcher.DefaultCategories()
	v.state.Preferences.Categories[0].Visible = false
	v.mu.Unlock()
	v.ResetSearch()
	tt.Frame()
	if v.category != "favorites" {
		t.Fatalf("hidden all should fall back to first visible: %s", v.category)
	}
}

func TestSingleCategoryAndOrdinaryControlsKeepNormalTab(t *testing.T) {
	f := fixture(3)
	f.state.Preferences.Categories = launcher.DefaultCategories()
	for i := range f.state.Preferences.Categories {
		f.state.Preferences.Categories[i].Visible = i == 0
	}
	v := testPad(f, Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	if tt.HasText("Tab 切换 · ⇧Tab 返回") {
		t.Fatal("single category displayed an ineffective switch hint")
	}
	tt.Type("工具")
	tt.Key(0, ui.KeyTab)
	if v.query != "工具" || v.category != "all" || !tt.Focused("清空搜索") {
		t.Fatal("single category did not preserve ordinary Tab focus")
	}
	v.mu.Lock()
	v.state.Preferences.Categories = launcher.DefaultCategories()
	v.mu.Unlock()
	tt.Frame()
	if err := tt.Click("管理应用"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyTab)
	if v.category != "all" || !tt.Focused("收起启动台") {
		t.Fatal("ordinary control Tab changed categories")
	}
}

func TestCategoryTabRespectsCompositionAndModal(t *testing.T) {
	v := testPad(fixture(8), Callbacks{})
	defer v.Close()
	modal := false
	name := ""
	tt := ui.NewTester(func(c *ui.Context) {
		v.View(c)
		ui.Modal(c, &modal, func() { ui.TextInput(c, &name).Label("名称").AutoFocus(); ui.Button(c, "保存") })
	}, 900, 650)
	tt.Compose("工具", 2)
	if !v.composing {
		t.Fatal("fixture input is not composing")
	}
	tt.Key(0, ui.KeyTab)
	if v.category != "all" || v.focus != "search" {
		t.Fatal("Tab interrupted input-method composition")
	}
	tt.Compose("", 0)
	modal = true
	tt.Frame()
	tt.Key(0, ui.KeyTab)
	if v.category != "all" {
		t.Fatal("modal Tab changed launchpad category")
	}
	if !tt.Focused("保存") {
		t.Fatal("modal did not retain ordinary Tab navigation")
	}
}

type retryCatalog struct {
	mu       sync.Mutex
	state    launcher.State
	attempts int
	succeeds bool
	png      []byte
}

func (f *retryCatalog) State() (launcher.State, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.state
	state.Apps = append([]launcher.App(nil), state.Apps...)
	return state, nil
}
func (f *retryCatalog) Refresh() (launcher.State, error) { return f.State() }
func (f *retryCatalog) Launch(string) error              { return nil }
func (f *retryCatalog) Icon(string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.attempts++
	if !f.succeeds {
		return nil, fmt.Errorf("temporarily unavailable")
	}
	return f.png, nil
}

func TestIconFailureRetriesAfterReloadAndVersionChange(t *testing.T) {
	image := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	image.Set(0, 0, color.NRGBA{R: 200, A: 255})
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image); err != nil {
		t.Fatal(err)
	}
	f := &retryCatalog{state: fixture(1).state, png: encoded.Bytes()}
	f.state.Apps[0].Version = "1.0"
	events := make(chan struct{}, 8)
	v := New(f, Callbacks{Invalidate: func() { events <- struct{}{} }})
	defer v.Close()
	v.state, _ = f.State()
	v.loaded = true
	wait := func() {
		t.Helper()
		select {
		case <-events:
		case <-time.After(time.Second):
			t.Fatal("background update did not finish")
		}
	}
	v.bitmap("app-000")
	wait()
	v.mu.Lock()
	pending, failed := v.pending["app-000"], v.failed["app-000"]
	v.mu.Unlock()
	if pending || !failed {
		t.Fatalf("failure left pending=%v failed=%v", pending, failed)
	}
	v.bitmap("app-000") // Drawing another frame must not create a retry loop.
	f.mu.Lock()
	attempts := f.attempts
	f.succeeds = true
	f.mu.Unlock()
	if attempts != 1 {
		t.Fatalf("failure retried without a refresh: %d attempts", attempts)
	}
	v.Reload()
	wait()
	v.bitmap("app-000")
	wait()
	first := v.bitmap("app-000")
	if first == nil {
		t.Fatal("refresh did not retry the failed icon")
	}
	f.mu.Lock()
	f.state.Apps[0].Version = "2.0"
	f.mu.Unlock()
	v.Reload()
	wait()
	if bitmap := v.bitmap("app-000"); bitmap != nil {
		t.Fatal("updated application kept its stale icon")
	}
	wait()
	if bitmap := v.bitmap("app-000"); bitmap == nil || bitmap == first {
		t.Fatal("updated application icon was not replaced")
	}
	f.mu.Lock()
	attempts = f.attempts
	f.mu.Unlock()
	if attempts != 3 {
		t.Fatalf("icon attempts %d, want failure, retry, and version update", attempts)
	}
}

func TestPaginationClampsAfterFilteringAndResizing(t *testing.T) {
	f := fixture(150)
	f.state.Preferences.IconSize = 112
	v := testPad(f, Callbacks{})
	defer v.Close()
	v.page = 100
	tt := ui.NewTester(v.View, 700, 520)
	if v.page != v.pageCount-1 {
		t.Fatalf("page %d not clamped to %d", v.page, v.pageCount-1)
	}
	// At most seven page dots keep the glass pager within small windows.
	if r, ok := tt.Find("下一页"); !ok || r.X+r.W > 700 {
		t.Fatalf("pager overflows small window: %v found=%v", r, ok)
	}
	v.query = "工具 001"
	tt.Frame()
	if v.page != 0 || v.pageCount != 1 {
		t.Fatalf("filtered pagination page=%d pages=%d", v.page, v.pageCount)
	}
	v.query = ""
	v.page = 40
	tt.SetSize(1440, 900)
	if v.page >= v.pageCount {
		t.Fatalf("resized pagination page=%d pages=%d", v.page, v.pageCount)
	}
}

// This opt-in render uses the actual Mac catalog and wallpaper without opening
// a screen window. It is also the visual QA artifact for the native glass view.
func TestRenderNativePreview(t *testing.T) {
	if os.Getenv("GOPAD_RENDER_NATIVE") != "1" {
		t.Skip("set GOPAD_RENDER_NATIVE=1 to render the real native launchpad")
	}
	service, err := launcher.New(filepath.Join(t.TempDir(), "preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.State()
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("GOPAD_RENDER_SINGLE_CATEGORY") == "1" {
		state.Preferences.Categories = launcher.DefaultCategories()
		for i := range state.Preferences.Categories {
			state.Preferences.Categories[i].Visible = i == 0
		}
	}
	locale := os.Getenv("GOPAD_RENDER_LOCALE")
	if locale == "" {
		locale = "zh-CN"
	}
	v := New(service, Callbacks{Locale: func() string { return locale }})
	defer v.Close()
	v.state = state
	v.loaded = true
	if data, err := launcher.Wallpaper(); err == nil {
		if err := v.SetWallpaper(data); err != nil {
			t.Fatal(err)
		}
	}
	geometry := layout(1440, 900, float32(state.Preferences.IconSize))
	apps := appsFor(state, "")
	for _, app := range apps[:min(len(apps), geometry.Capacity)] {
		data, err := service.Icon(app.ID)
		if err != nil {
			continue
		}
		bitmap, err := ui.DecodeBitmap(data)
		if err == nil {
			v.icons[app.ID] = bitmap
		}
	}
	tt := ui.NewTester(v.View, 1440, 900)
	output := os.Getenv("GOPAD_RENDER_OUTPUT")
	if output == "" {
		output = "/tmp/gopad-native-render.png"
		if locale == "en" {
			output = "/tmp/gopad-native-render-en.png"
		}
	}
	f, err := os.Create(output)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
	t.Logf("rendered %d of %d real apps to %s", geometry.Capacity, len(apps), output)
}

// Inspect native icon alpha edges without opening a window. Low alpha includes
// the shadow; the 25–50% thresholds better describe the visible app-icon plate.
func TestInspectNativeIconInsets(t *testing.T) {
	if os.Getenv("GOPAD_INSPECT_NATIVE_ICONS") != "1" {
		t.Skip("set GOPAD_INSPECT_NATIVE_ICONS=1 for native asset measurements")
	}
	service, err := launcher.New(filepath.Join(t.TempDir(), "preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.State()
	if err != nil {
		t.Fatal(err)
	}
	for _, app := range state.Apps {
		if app.BundleID != "com.apple.AboutThisMacLauncher" && app.BundleID != "com.apple.airport.airportutility" && app.BundleID != "com.apple.ActivityMonitor" {
			continue
		}
		data, err := service.Icon(app.ID)
		if err != nil {
			t.Fatal(err)
		}
		bitmap, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		for _, threshold := range []uint32{1, 6553, 16384, 32768} {
			left, right := bitmap.Bounds().Max.X, -1
			for y := bitmap.Bounds().Min.Y; y < bitmap.Bounds().Max.Y; y++ {
				for x := bitmap.Bounds().Min.X; x < bitmap.Bounds().Max.X; x++ {
					_, _, _, alpha := bitmap.At(x, y).RGBA()
					if alpha >= threshold {
						left = min(left, x)
						right = max(right, x)
					}
				}
			}
			t.Logf("%s: width=%d alpha=%0.2f visible x=%d..%d left=%0.3f", app.Name, bitmap.Bounds().Dx(), float64(threshold)/65535, left, right, float64(left)/float64(bitmap.Bounds().Dx()))
		}
	}
}
