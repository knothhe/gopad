package launchview

import (
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/launcher"
)

type appActionCall struct{ action, id, target string }

type actionFixture struct {
	*fixtureCatalog
	calls chan appActionCall
	err   error
	block <-chan struct{}
}

func (f *actionFixture) ApplyAppAction(action, id, target string) (launcher.State, error) {
	f.calls <- appActionCall{action, id, target}
	if f.block != nil {
		<-f.block
	}
	state := f.state
	if action == "hide" {
		state.Preferences.Hidden = append(slices.Clone(state.Preferences.Hidden), id)
	}
	if action == "favorite-add" {
		state.Preferences.Favorites = []string{id}
	}
	return state, f.err
}

func actionPad(t *testing.T) (*Launchpad, *actionFixture) {
	t.Helper()
	f := &actionFixture{fixtureCatalog: fixture(2), calls: make(chan appActionCall, 8)}
	v := New(f, Callbacks{})
	v.state, v.loaded = f.state, true
	t.Cleanup(v.Close)
	return v, f
}

func awaitAction(t *testing.T, v *Launchpad, f *actionFixture) appActionCall {
	t.Helper()
	var call appActionCall
	select {
	case call = <-f.calls:
	case <-time.After(time.Second):
		t.Fatal("app action was not sent to backend")
	}
	deadline := time.Now().Add(time.Second)
	for v.actionPending() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if v.actionPending() {
		t.Fatal("app action did not finish")
	}
	return call
}

func TestAppContextMenuMutationsAndUndo(t *testing.T) {
	v, f := actionPad(t)
	f.state.Preferences.Categories = []launcher.Category{{ID: "tools", Name: "Tools", Visible: true}}
	v.state = f.state
	tt := ui.NewTester(func(c *ui.Context) {
		tile := ui.ButtonBase(c).Size(150, 100).Label("Test app")
		v.bindAppActions(tile, f.state.Apps[0])
	}, 300, 200)
	if err := tt.RightClick("Test app"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), "加入收藏") || !slices.Contains(tt.Menu(), "隐藏应用") {
		t.Fatalf("missing quick app actions: %v", tt.Menu())
	}
	if err := tt.ChooseMenuItem("添加到分区", "Tools"); err != nil {
		t.Fatal(err)
	}
	if call := awaitAction(t, v, f); call != (appActionCall{"category", "app-000", "tools"}) {
		t.Fatalf("category action = %+v", call)
	}
	tt.Frame()
	if err := tt.RightClick("Test app"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("隐藏应用"); err != nil {
		t.Fatal(err)
	}
	if call := awaitAction(t, v, f); call.action != "hide" || call.id != "app-000" {
		t.Fatalf("hide action = %+v", call)
	}
	v.mu.Lock()
	undoID := v.actions.undoID
	v.mu.Unlock()
	if undoID != "app-000" {
		t.Fatalf("hidden app undo = %q", undoID)
	}
	v.undoHidden()
	if call := awaitAction(t, v, f); call != (appActionCall{"restore", "app-000", ""}) {
		t.Fatalf("restore action = %+v", call)
	}
}

func TestAppDragReordersWithoutLaunching(t *testing.T) {
	v, f := actionPad(t)
	clicks := 0
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Gap(20).Children(func() {
			for i, app := range f.state.Apps {
				tile := ui.ButtonBase(c.Key(app.ID)).Size(100, 100).Label(app.ID).OnClick(func() { clicks++ })
				v.bindAppActions(tile, f.state.Apps[i])
			}
		})
	}, 300, 200)
	from, _ := tt.Find("app-000")
	to, _ := tt.Find("app-001")
	tt.Press(from.X+10, from.Y+10)
	tt.Move(from.X+12, from.Y+11)
	tt.Release(from.X+12, from.Y+11)
	if clicks != 1 {
		t.Fatal("small pointer movement prevented ordinary click")
	}
	tt.Press(from.X+10, from.Y+10)
	tt.Move(from.X+30, from.Y+30)
	tt.Move(to.X+20, to.Y+20)
	tt.Release(to.X+20, to.Y+20)
	if call := awaitAction(t, v, f); call != (appActionCall{"reorder", "app-000", "app-001"}) {
		t.Fatalf("drop action = %+v", call)
	}
	if clicks != 1 {
		t.Fatal("drop also launched the source app")
	}
}

func TestAppDragToCategoryAndSearchDoesNotReorder(t *testing.T) {
	v, f := actionPad(t)
	v.query = "工具"
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Gap(20).Children(func() {
			for _, app := range f.state.Apps {
				tile := ui.ButtonBase(c.Key(app.ID)).Size(80, 80).Label(app.ID)
				v.bindAppActions(tile, app)
			}
			tab := ui.ButtonBase(c.Key("tools")).Size(80, 80).Label("Tools")
			v.categoryDropTarget(tab, "tools")
		})
	}, 320, 200)
	from, _ := tt.Find("app-000")
	to, _ := tt.Find("app-001")
	tt.Press(from.X+10, from.Y+10)
	tt.Move(to.X+20, to.Y+20)
	tt.Release(to.X+20, to.Y+20)
	select {
	case call := <-f.calls:
		t.Fatalf("reordered filtered search results: %+v", call)
	default:
	}
	category, _ := tt.Find("Tools")
	tt.Press(from.X+10, from.Y+10)
	tt.Move(category.X+20, category.Y+20)
	tt.Release(category.X+20, category.Y+20)
	if call := awaitAction(t, v, f); call != (appActionCall{"category", "app-000", "tools"}) {
		t.Fatalf("category drop action = %+v", call)
	}
}

func TestFailedAppMutationKeepsStateAndHasNoUndo(t *testing.T) {
	v, f := actionPad(t)
	f.err = errors.New("disk is full")
	v.applyAppAction("hide", "app-000", "")
	awaitAction(t, v, f)
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.errKey != "native.error.action" || v.actions.undoID != "" || len(v.state.Preferences.Hidden) != 0 {
		t.Fatalf("failed mutation changed state: error=%q undo=%q hidden=%v", v.errKey, v.actions.undoID, v.state.Preferences.Hidden)
	}
}

func TestEmptyClickDismissesOnlyLauncherBackground(t *testing.T) {
	v, f := actionPad(t)
	v.state.Preferences.DismissOnEmptyClick = true
	var hidden atomic.Int32
	v.callbacks.Hide = func() { hidden.Add(1) }
	tt := ui.NewTester(v.View, 900, 650)
	tt.ClickAt(12, 300)
	if hidden.Load() != 1 {
		t.Fatal("background click did not dismiss launcher")
	}
	for _, label := range []string{v.t("native.search"), v.t("native.previousPage")} {
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		if hidden.Load() != 1 {
			t.Fatalf("clicking control %q also dismissed launcher", label)
		}
	}
	// Decorative chrome belongs to its control even outside the actual text
	// input or pager buttons.
	for _, point := range [][2]float32{{295, 64}, {403, 597}} {
		tt.ClickAt(point[0], point[1])
		if hidden.Load() != 1 {
			t.Fatalf("clicking control chrome at %v dismissed launcher", point)
		}
	}
	if err := tt.Click(v.t("native.search")); err != nil {
		t.Fatal(err)
	}
	tt.Type("工具")
	if err := tt.Click(v.t("native.clearSearch")); err != nil {
		t.Fatal(err)
	}
	if hidden.Load() != 1 {
		t.Fatal("clearing search also dismissed launcher")
	}
	if err := tt.RightClick(f.state.Apps[0].Name); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(v.t("native.menu.favorite")); err != nil {
		t.Fatal(err)
	}
	awaitAction(t, v, f)
	if hidden.Load() != 1 {
		t.Fatal("app context menu dismissed launcher")
	}
}

func TestEmptyClickCanBeDisabledAndDragNeverDismisses(t *testing.T) {
	v, _ := actionPad(t)
	var hidden atomic.Int32
	v.callbacks.Hide = func() { hidden.Add(1) }
	tt := ui.NewTester(v.View, 900, 650)
	tt.ClickAt(12, 300)
	if hidden.Load() != 0 {
		t.Fatal("background dismissed despite disabled preference")
	}
	v.state.Preferences.DismissOnEmptyClick = true
	tt.Frame()
	from, ok := tt.Find("工具 000")
	if !ok {
		t.Fatal("missing draggable app")
	}
	tt.Press(from.X+10, from.Y+10)
	tt.Move(from.X+35, from.Y+35)
	tt.Move(12, 300)
	tt.Release(12, 300)
	if hidden.Load() != 0 {
		t.Fatal("app drag released on background dismissed launcher")
	}
}

func TestAppErrorFeedbackKeepsUndoWithoutOverlappingToasts(t *testing.T) {
	v, f := actionPad(t)
	v.actions.undoID, v.actions.undoName = "app-000", "工具 000"
	v.errKey, v.errDetail = "native.error.action", "disk is full"
	tt := ui.NewTester(v.View, 700, 520)
	if tt.HasText(v.t("native.hidden", "name", "工具 000")) {
		t.Fatal("standalone hide toast overlaps the active error surface")
	}
	if !tt.HasText(v.t("native.error.action", "detail", "disk is full")) {
		t.Fatal("app mutation error is not visible")
	}
	if err := tt.Click(v.t("native.undo")); err != nil {
		t.Fatalf("error surface lost undo: %v", err)
	}
	if call := awaitAction(t, v, f); call.action != "restore" || call.id != "app-000" {
		t.Fatalf("error-surface undo = %+v", call)
	}
}

func TestAppActionsQueueBehindSlowPreferenceSave(t *testing.T) {
	v, f := actionPad(t)
	release := make(chan struct{})
	f.block = release
	v.applyAppAction("favorite", "app-000", "")
	select {
	case first := <-f.calls:
		if first.action != "favorite" {
			t.Fatalf("first action = %+v", first)
		}
	case <-time.After(time.Second):
		t.Fatal("first app action did not begin")
	}
	v.applyAppAction("hide", "app-001", "")
	close(release)
	if second := awaitAction(t, v, f); second != (appActionCall{"hide", "app-001", ""}) {
		t.Fatalf("queued action = %+v", second)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.actions.pending || len(v.actions.queue) != 0 || v.actions.undoID != "app-001" {
		t.Fatalf("action queue did not drain: %+v", v.actions)
	}
}

func TestCloseDrainsNativeAppActionsWithoutInvalidatingClosedView(t *testing.T) {
	v, f := actionPad(t)
	release := make(chan struct{})
	f.block = release
	var invalidations atomic.Int32
	v.callbacks.Invalidate = func() { invalidations.Add(1) }
	v.applyAppAction("favorite", "app-000", "")
	select {
	case <-f.calls:
	case <-time.After(time.Second):
		t.Fatal("app save did not begin")
	}
	v.applyAppAction("hide", "app-001", "")
	closed := make(chan struct{})
	go func() { v.Close(); close(closed) }()
	select {
	case <-v.stop:
	case <-time.After(time.Second):
		t.Fatal("Close did not stop the view")
	}
	select {
	case <-closed:
		t.Fatal("Close returned before pending app edits finished")
	default:
	}
	v.applyAppAction("hide", "app-000", "")
	close(release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after app edits saved")
	}
	if invalidations.Load() != 0 {
		t.Fatal("app action invalidated view after it stopped")
	}
	select {
	case second := <-f.calls:
		if second != (appActionCall{"hide", "app-001", ""}) {
			t.Fatalf("close lost queued app edit: %+v", second)
		}
	default:
		t.Fatal("queued app edit was not saved on close")
	}
	select {
	case extra := <-f.calls:
		t.Fatalf("app action was accepted after Close began: %+v", extra)
	default:
	}
}

func TestRapidRepeatedFavoriteDropsQueueAdditionsInsteadOfToggles(t *testing.T) {
	v, f := actionPad(t)
	release := make(chan struct{})
	f.block = release
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Gap(20).Children(func() {
			source := ui.ButtonBase(c.Key("source")).Size(100, 100).Label("Source app")
			v.bindAppActions(source, f.state.Apps[0])
			target := ui.ButtonBase(c.Key("favorite-target")).Size(100, 100).Label("Favorites")
			v.categoryDropTarget(target, "favorites")
		})
	}, 300, 200)
	from, _ := tt.Find("Source app")
	to, _ := tt.Find("Favorites")
	drop := func() {
		tt.Press(from.X+10, from.Y+10)
		tt.Move(to.X+20, to.Y+20)
		tt.Release(to.X+20, to.Y+20)
	}
	drop()
	select {
	case first := <-f.calls:
		if first.action != "favorite-add" {
			t.Fatalf("favorite drop sent toggle: %+v", first)
		}
	case <-time.After(time.Second):
		t.Fatal("first favorite drop was not saved")
	}
	drop()
	close(release)
	if second := awaitAction(t, v, f); second.action != "favorite-add" || second.id != "app-000" {
		t.Fatalf("queued favorite drop sent toggle: %+v", second)
	}
	state, _, _ := v.snapshot()
	if !slices.Equal(state.Preferences.Favorites, []string{"app-000"}) {
		t.Fatalf("repeated drops did not retain favorite: %v", state.Preferences.Favorites)
	}
}
