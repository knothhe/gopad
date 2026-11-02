// Package launchview is gopad's native, full-screen application launcher.
// Editing lives in a separate management window, leaving only the applications
// and small Liquid Glass controls on this surface.
package launchview

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/appsearch"
	"github.com/knothhe/gopad/internal/i18n"
	"github.com/knothhe/gopad/internal/launcher"
	"github.com/knothhe/gopad/internal/uikit"
)

type Catalog interface {
	State() (launcher.State, error)
	Refresh() (launcher.State, error)
	Icon(string) ([]byte, error)
	Launch(string) error
}

type Callbacks struct {
	Hide        func()
	OpenManager func()
	Invalidate  func()
	Locale      func() string
}

type iconJob struct {
	id    string
	epoch uint64
}

type Launchpad struct {
	catalog               Catalog
	callbacks             Callbacks
	mu                    sync.Mutex
	state                 launcher.State
	loaded                bool
	errKey                string
	errDetail             string
	categorySaveError     string
	categoryRestored      bool
	lastCategoryIntent    string
	lastCategoryRevision  uint64
	lastCategoryProcessed uint64
	saveLastCategory      func(string) error
	saveWake              chan struct{}
	saveDone              chan struct{}
	icons                 map[string]*ui.Bitmap
	pending               map[string]bool
	failed                map[string]bool
	iconEpoch             uint64
	jobs                  chan iconJob
	stop                  chan struct{}
	closeOnce             sync.Once
	busy                  bool
	generation            uint64
	wallpaper             *ui.Bitmap
	theme                 *ui.Theme
	query                 string
	category              string
	categories            []launcher.Category
	composing             bool
	categoryDirection     int
	slideReset            bool
	slides                slideDeck
	pageDirection         int
	page                  int
	selected              int
	focus                 string
	columns               int
	pageSize              int
	pageCount             int
	filtered              []launcher.App
	lastWheel             time.Time
	scroll                pageScroll
}

func New(catalog Catalog, callbacks Callbacks) *Launchpad {
	theme := ui.DarkTheme()
	theme.Background = ui.Hex("#202c43")
	theme.Text = ui.RGB(255, 255, 255)
	theme.TextMuted = ui.RGBA(255, 255, 255, .65)
	theme.Focus = ui.RGBA(255, 255, 255, .48)
	v := &Launchpad{catalog: catalog, callbacks: callbacks, icons: make(map[string]*ui.Bitmap), pending: make(map[string]bool), failed: make(map[string]bool), jobs: make(chan iconJob, 256), stop: make(chan struct{}), saveWake: make(chan struct{}, 1), saveDone: make(chan struct{}), theme: theme, focus: "search", category: "all", selected: -1, state: launcher.State{Preferences: launcher.Preferences{IconSize: 80}}}
	if saver, ok := catalog.(categorySaver); ok {
		v.saveLastCategory = saver.SetLastCategory
	}
	go v.loadIcons()
	go v.saveCategories()
	return v
}

func (v *Launchpad) Close() { v.closeOnce.Do(func() { close(v.stop) }); <-v.saveDone }
func (v *Launchpad) invalidate() {
	if v.callbacks.Invalidate != nil {
		v.callbacks.Invalidate()
	}
}

// Reload asynchronously reads the latest catalog and preferences. It is safe
// when called from a native activation callback or after management closes.
func (v *Launchpad) Reload() {
	v.mu.Lock()
	v.generation++
	generation := v.generation
	v.mu.Unlock()
	go func() {
		state, err := v.catalog.Refresh()
		v.mu.Lock()
		if generation == v.generation {
			if err != nil {
				v.errKey = "native.error.load"
				v.errDetail = err.Error()
			} else {
				// A fresh catalog retries transient failures and supersedes old
				// in-flight icon work. Preserve valid icons across ordinary opens.
				oldApps := make(map[string]launcher.App, len(v.state.Apps))
				for _, app := range v.state.Apps {
					oldApps[app.ID] = app
				}
				newIcons := make(map[string]*ui.Bitmap, len(v.icons))
				for _, app := range state.Apps {
					old, ok := oldApps[app.ID]
					if ok && old.Path == app.Path && old.Version == app.Version && old.BundleID == app.BundleID {
						if bitmap := v.icons[app.ID]; bitmap != nil {
							newIcons[app.ID] = bitmap
						}
					}
				}
				v.icons = newIcons
				v.iconEpoch++
				v.pending = make(map[string]bool)
				v.failed = make(map[string]bool)
				if v.lastCategoryIntent != "" {
					state.Preferences.LastCategory = v.lastCategoryIntent
				}
				v.state = state
				v.loaded = true
				v.errKey = ""
				v.errDetail = ""
			}
		}
		v.mu.Unlock()
		v.invalidate()
	}()
}

// ResetSearch is called on the UI thread each time the launchpad is opened.
func (v *Launchpad) ResetSearch() {
	v.query = ""
	v.page = 0
	v.selected = -1
	v.focus = "search"
	v.slideReset = true
}
func (v *Launchpad) SetWallpaper(data []byte) error {
	bitmap, err := ui.DecodeBitmap(data)
	if err != nil {
		return err
	}
	v.wallpaper = bitmap
	return nil
}
func (v *Launchpad) locale() string {
	if v.callbacks.Locale == nil {
		return "zh-CN"
	}
	return i18n.Resolve(v.callbacks.Locale(), "zh-CN")
}
func (v *Launchpad) t(key string, variables ...string) string {
	return i18n.T(v.locale(), key, variables...)
}
func (v *Launchpad) snapshot() (launcher.State, bool, string) {
	v.mu.Lock()
	state, loaded, key, detail := v.state, v.loaded, v.errKey, v.errDetail
	if v.categorySaveError != "" {
		key = "native.error.categorySave"
		detail = v.categorySaveError
	}
	v.mu.Unlock()
	message := ""
	if key != "" {
		message = v.t(key, "detail", detail)
	}
	return state, loaded, message
}
func (v *Launchpad) bitmap(id string) *ui.Bitmap {
	v.mu.Lock()
	defer v.mu.Unlock()
	bitmap := v.icons[id]
	if bitmap == nil && !v.pending[id] && !v.failed[id] {
		v.pending[id] = true
		select {
		case v.jobs <- iconJob{id: id, epoch: v.iconEpoch}:
		default:
			delete(v.pending, id)
		}
	}
	return bitmap
}
func (v *Launchpad) loadIcons() {
	for {
		select {
		case <-v.stop:
			return
		case job := <-v.jobs:
			v.mu.Lock()
			current := job.epoch == v.iconEpoch && v.pending[job.id]
			v.mu.Unlock()
			if !current {
				continue
			}
			data, err := v.catalog.Icon(job.id)
			var bitmap *ui.Bitmap
			if err == nil {
				bitmap, err = ui.DecodeBitmap(data)
			}
			v.mu.Lock()
			current = job.epoch == v.iconEpoch
			if current {
				delete(v.pending, job.id)
				if err == nil {
					v.icons[job.id] = bitmap
				} else {
					v.failed[job.id] = true
				}
			}
			v.mu.Unlock()
			if current {
				v.invalidate()
			}
		}
	}
}

func appsFor(state launcher.State, query string) []launcher.App {
	return appsForCategory(state, query, launcher.Category{ID: "all"})
}
func appsForCategory(state launcher.State, query string, category launcher.Category) []launcher.App {
	rank := make(map[string]int, len(state.Preferences.Order))
	for i, id := range state.Preferences.Order {
		rank[id] = i
	}
	apps := make([]launcher.App, 0, len(state.Apps))
	for _, app := range state.Apps {
		if !launcher.CategoryContains(category, app, state.Preferences) {
			continue
		}
		if !appsearch.Match(app.Name+" "+app.BundleID+" "+app.Path, query) {
			continue
		}
		apps = append(apps, app)
	}
	sort.SliceStable(apps, func(i, j int) bool {
		a, aOK := rank[apps[i].ID]
		b, bOK := rank[apps[j].ID]
		if aOK != bOK {
			return aOK
		}
		if aOK && a != b {
			return a < b
		}
		return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name)
	})
	return apps
}

type gridLayout struct {
	Columns, Rows, Capacity int
	CellW, CellH, X, Y      float32
}

func layout(width, height, size float32) gridLayout {
	cellW, cellH := size+66, size+60
	columns := max(3, min(9, int((width-2*max(float32(64), width*.08))/cellW)))
	rows := max(1, min(6, int((height-280)/cellH)))
	return gridLayout{Columns: columns, Rows: rows, Capacity: columns * rows, CellW: cellW, CellH: cellH, X: (width - float32(columns)*cellW) / 2, Y: 176 + max(0, (height-280-float32(rows)*cellH)/2)}
}

// NSWorkspace's standard 256px icons have a visible plate from x=25 to
// x=230 at 25–50% alpha; fainter pixels outside it are the drop shadow.
// Align controls to that optical plate, not the wider tile or PNG canvas.
const iconOpticalInsetRatio float32 = 25.0 / 256.0

func iconOpticalInset(size float32) float32 { return size * iconOpticalInsetRatio }
func categoryGeometry(geometry gridLayout, size float32) (x, y, maxWidth float32) {
	inset := (geometry.CellW-size)/2 + iconOpticalInset(size)
	return geometry.X + inset, geometry.Y - 52, float32(geometry.Columns)*geometry.CellW - 2*inset
}

func (v *Launchpad) selectCategory(id string) {
	direction := 1
	oldIndex, newIndex := -1, -1
	for i, category := range v.categories {
		if category.ID == v.category {
			oldIndex = i
		}
		if category.ID == id {
			newIndex = i
		}
	}
	if oldIndex >= 0 && newIndex >= 0 && newIndex < oldIndex {
		direction = -1
	}
	v.selectCategoryDirection(id, direction)
}
func (v *Launchpad) selectCategoryDirection(id string, direction int) {
	if id == v.category {
		if !v.categoryRestored {
			v.categoryRestored = true
			v.rememberCategory(id)
		}
		return
	}
	for _, category := range v.categories {
		if category.ID == id {
			v.category = id
			v.categoryDirection = direction
			v.categoryRestored = true
			v.rememberCategory(id)
			v.query = ""
			v.page = 0
			v.selected = -1
			v.focus = "search"
			return
		}
	}
}
func (v *Launchpad) restoreCategory(saved string, loaded bool) {
	if v.categoryRestored || !loaded {
		return
	}
	candidate := saved
	if candidate == "" {
		candidate = "all"
	}
	found := false
	for _, category := range v.categories {
		if category.ID == candidate {
			found = true
			break
		}
	}
	if !found && len(v.categories) > 0 {
		candidate = v.categories[0].ID
	}
	v.category = candidate
	v.categoryRestored = true
	if candidate != saved {
		v.rememberCategory(candidate)
	} else {
		v.mu.Lock()
		v.lastCategoryIntent = candidate
		v.mu.Unlock()
	}
}
func (v *Launchpad) ensureCategory(categories []launcher.Category) {
	v.categories = categories
	for _, category := range categories {
		if category.ID == v.category {
			return
		}
	}
	if len(categories) > 0 {
		v.selectCategory(categories[0].ID)
	}
}
func (v *Launchpad) cycleCategory(delta int) {
	if len(v.categories) == 0 {
		return
	}
	index := 0
	for i, category := range v.categories {
		if category.ID == v.category {
			index = i
			break
		}
	}
	v.selectCategoryDirection(v.categories[(index+delta%len(v.categories)+len(v.categories))%len(v.categories)].ID, delta)
}
func (v *Launchpad) currentCategory() launcher.Category {
	for _, category := range v.categories {
		if category.ID == v.category {
			return category
		}
	}
	return launcher.Category{ID: "all"}
}

func (v *Launchpad) changePage(page int) {
	count := max(1, v.pageCount)
	if page != v.page {
		v.pageDirection = 1
		if page < v.page {
			v.pageDirection = -1
		}
	}
	v.page = (page%count + count) % count
	v.selected = -1
	v.focus = "search"
}
func (v *Launchpad) moveSelection(delta int) {
	if len(v.filtered) == 0 {
		return
	}
	index := v.selected
	// FocusBind also follows Tab and pointer focus. Navigation starts from
	// the actual focused app, even when no arrow selection has been made.
	if v.focus == "search" {
		index = -1
	} else {
		for i, app := range v.filtered {
			if app.ID == v.focus {
				index = i
				break
			}
		}
	}
	if index < 0 {
		index = v.page * v.pageSize
	} else {
		index += delta
	}
	index = max(0, min(len(v.filtered)-1, index))
	v.selected = index
	v.page = index / v.pageSize
	v.focus = v.filtered[index].ID
}
func (v *Launchpad) launch(app launcher.App) {
	v.mu.Lock()
	if v.busy {
		v.mu.Unlock()
		return
	}
	v.busy = true
	v.mu.Unlock()
	go func() {
		err := v.catalog.Launch(app.ID)
		v.mu.Lock()
		v.busy = false
		if err != nil {
			v.errKey = "native.error.launch"
			v.errDetail = err.Error()
		}
		v.mu.Unlock()
		if err == nil && v.callbacks.Hide != nil {
			v.callbacks.Hide()
		}
		v.invalidate()
	}()
}
func (v *Launchpad) activateSelection() {
	if v.slides.dragging || len(v.filtered) == 0 {
		return
	}
	index := v.selected
	if index < 0 || index >= len(v.filtered) {
		index = v.page * v.pageSize
	}
	v.launch(v.filtered[index])
}
func (v *Launchpad) escape() {
	if v.callbacks.Hide != nil {
		v.callbacks.Hide()
	}
}

func (v *Launchpad) View(c *ui.Context) {
	c.SetTheme(v.theme)
	width, height := c.Size()
	state, loaded, errText := v.snapshot()
	size := float32(state.Preferences.IconSize)
	if size < 48 || size > 112 {
		size = 80
	}
	geometry := layout(width, height, size)
	v.columns, v.pageSize = geometry.Columns, geometry.Capacity
	v.categories = launcher.VisibleCategories(state.Preferences)
	v.restoreCategory(state.Preferences.LastCategory, loaded)
	v.ensureCategory(v.categories)
	v.filtered = appsForCategory(state, v.query, v.currentCategory())
	v.pageCount = max(1, (len(v.filtered)+v.pageSize-1)/v.pageSize)
	v.page = max(0, min(v.page, v.pageCount-1))
	if v.selected >= len(v.filtered) {
		v.selected = -1
		v.focus = "search"
	}
	if c.Shortcut(ui.Cmd, ui.KeyComma) && v.callbacks.OpenManager != nil {
		v.callbacks.OpenManager()
	}
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		v.focus = "search"
	}
	if !v.composing && c.Shortcut(0, ui.KeyEscape) {
		v.escape()
	}
	if c.Shortcut(0, ui.KeyLeft) {
		v.moveSelection(-1)
	}
	if c.Shortcut(0, ui.KeyRight) {
		v.moveSelection(1)
	}
	if c.Shortcut(0, ui.KeyUp) {
		v.moveSelection(-v.columns)
	}
	if c.Shortcut(0, ui.KeyDown) {
		v.moveSelection(v.columns)
	}
	if c.Shortcut(0, ui.KeyPageDown) {
		v.changePage(v.page + 1)
	}
	if c.Shortcut(0, ui.KeyPageUp) {
		v.changePage(v.page - 1)
	}
	if c.Shortcut(0, ui.KeyEnter) {
		v.activateSelection()
	}
	root := ui.Box(c).Fill().Clip().HandleInput(func(ev ui.InputEvent) bool {
		return v.handlePageScroll(ev, ev.Time)
	})
	frames := v.slideFrames(c, width, height, size, loaded)
	root.Children(func() {
		ui.Box(c).Fill().LinearGradient(ui.LinearGradient{From: ui.Hex("#34436e"), To: ui.Hex("#233f43"), Angle: 135})
		if v.wallpaper != nil {
			ui.Image(c, v.wallpaper).Absolute().Top(0).Left(0).Size(width, height).Fit(ui.Cover)
		} else {
			ui.Box(c).Absolute().Top(0).Left(0).Size(width, height).LinearGradient(ui.LinearGradient{From: ui.RGBA(145, 115, 166, .65), To: ui.Transparent, Angle: 145})
		}
		uikit.BackdropBlur(c, 16).Absolute().Top(0).Left(0).Size(width, height)
		ui.Box(c).Absolute().Top(0).Left(0).Size(width, height).Background(ui.RGBA(0, 0, 0, .28)).PassThrough()
		categoryArea := ui.Box(c.Key("category-area")).Absolute().Top(0).Left(0).Size(width, height).PassThrough()
		categoryArea.Children(func() {
			uikit.Pill(c, 320, 44, uikit.Regular).Absolute().Top(42).Left((width-320)/2).Padding(0, 15).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, searchIcon).Size(16, 16).TextColor(ui.RGBA(255, 255, 255, .7))
				input := ui.TextInputBase(c.Key("search"), &v.query).Grow(1).MinWidth(0).Height(28).FontSize(14).Placeholder(v.t("native.search")).Label(v.t("native.search")).FocusBind(&v.focus, "search").AutoFocus().OnTextKey(func(k ui.TextKey) bool {
					if k.Key == ui.KeyTab && (k.Mods == 0 || k.Mods == ui.Shift) {
						if v.composing {
							return true
						}
						if len(v.categories) <= 1 {
							return false
						}
						k.SetText("")
						if k.Mods == ui.Shift {
							v.cycleCategory(-1)
						} else {
							v.cycleCategory(1)
						}
						return true
					}
					if v.composing || k.Mods != 0 {
						return false
					}
					if k.Key == ui.KeyEscape {
						v.escape()
						return true
					}
					if k.Key == ui.KeyDown {
						v.moveSelection(v.columns)
						return true
					}
					return false
				})
				v.composing = input.Composing()
				input.OnChange(func() { v.page = 0; v.selected = -1 }).OnSubmit(v.activateSelection)
				if v.query != "" {
					ui.ButtonBase(c.Key("clear-search")).Size(20, 20).Center().Radius(10).Label(v.t("native.clearSearch")).Children(func() { ui.Icon(c, closeIcon).Size(11, 11) }).OnClick(func() { v.query = ""; v.page = 0; v.selected = -1; v.focus = "search" })
				}
			})
			if len(v.categories) > 1 {
				tabs := make([]uikit.Tab, 0, len(v.categories))
				for _, category := range v.categories {
					label := category.Name
					switch category.ID {
					case "all", "favorites", "system", "hidden":
						label = v.t("category." + category.ID)
					}
					tabs = append(tabs, uikit.Tab{ID: category.ID, Label: label, AccessibleLabel: v.t("native.ui.category", "name", label)})
				}
				tabX, tabY, tabMaxWidth := categoryGeometry(geometry, size)
				uikit.Tabs(c.Key("categories"), tabs, v.category, tabMaxWidth, v.selectCategory, uikit.TabsLabels{Group: v.t("native.ui.categories"), Previous: v.t("native.ui.previousCategory"), Next: v.t("native.ui.nextCategory")}).Absolute().Top(tabY).Left(tabX)
			}
			for _, frame := range frames {
				v.renderScene(c, frame, geometry, width, height, size)
			}
		})
		if !v.composing && len(v.categories) > 1 {
			categoryArea.OnShortcut(0, ui.KeyTab, func() { v.cycleCategory(1) })
			categoryArea.OnShortcut(ui.Shift, ui.KeyTab, func() { v.cycleCategory(-1) })
			categoryArea.OnShortcut(ui.Ctrl, ui.KeyTab, func() { v.cycleCategory(1) })
			categoryArea.OnShortcut(ui.Ctrl|ui.Shift, ui.KeyTab, func() { v.cycleCategory(-1) })
		}

		visiblePages := min(7, v.pageCount)
		pagerStart := max(0, min(v.page-visiblePages/2, v.pageCount-visiblePages))
		pagerWidth := float32(80 + visiblePages*19)
		uikit.Pill(c, pagerWidth, 34, uikit.Clear).Absolute().Bottom(36).Left((width-pagerWidth)/2).Padding(0, 9).Gap(3).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
			ui.ButtonBase(c.Key("previous-page")).Size(24, 26).Center().Radius(12).Label(v.t("native.previousPage")).Disabled(v.pageCount <= 1).Children(func() { ui.Icon(c, leftIcon).Size(13, 13) }).OnClick(func() { v.changePage(v.page - 1) })
			for page := pagerStart; page < pagerStart+visiblePages; page++ {
				target := page
				ui.ButtonBase(c.Key(fmt.Sprintf("page-%d", page))).Size(16, 24).Center().Label(v.t("native.page", "number", fmt.Sprint(page+1))).Children(func() {
					color := ui.RGBA(255, 255, 255, .38)
					if target == v.page {
						color = ui.RGB(255, 255, 255)
					}
					dot := ui.Box(c.Key("dot")).Size(6, 6).Radius(3)
					dot.Background(dot.AnimateColor("active", color, ui.Animation{Duration: 160 * time.Millisecond}))
				}).OnClick(func() { v.changePage(target) })
			}
			ui.ButtonBase(c.Key("next-page")).Size(24, 26).Center().Radius(12).Label(v.t("native.nextPage")).Disabled(v.pageCount <= 1).Children(func() { ui.Icon(c, rightIcon).Size(13, 13) }).OnClick(func() { v.changePage(v.page + 1) })
		})
		ui.Row(c).Absolute().Right(38).Bottom(34).Gap(10).Children(func() {
			uikit.IconButton(c, "manage", v.t("native.manage"), settingsIcon).OnClick(func() {
				if v.callbacks.OpenManager != nil {
					v.callbacks.OpenManager()
				}
			})
			uikit.IconButton(c, "hide", v.t("native.hide"), closeIcon).OnClick(func() {
				if v.callbacks.Hide != nil {
					v.callbacks.Hide()
				}
			})
		})
		if len(v.categories) > 1 {
			alpha := float32(.55)
			if c.Preferences().HighContrast {
				alpha = .85
			}
			ui.Text(c, v.t("native.categoryHint")).Absolute().Left(40).Bottom(47).FontSize(11).TextColor(ui.RGBA(255, 255, 255, alpha))
		}
		if errText != "" {
			uikit.Surface(c, 16, uikit.Regular).Absolute().Bottom(88).Left(max(float32(30), (width-480)/2)).Width(min(float32(480), width-60)).Padding(12, 18).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, errText).Grow(1).FontSize(12).MaxLines(2)
				if v.categorySaveFailed() {
					ui.ButtonBase(c.Key("retry-category-save")).Padding(5, 9).Radius(7).Background(ui.RGBA(255, 255, 255, .1)).Label(v.t("native.error.retry")).Children(func() { ui.Text(c, v.t("native.error.retry")).FontSize(12) }).OnClick(v.retryCategorySave)
				}
			})
		}
	})
}

func (v *Launchpad) renderScene(c *ui.Context, frame sceneFrame, geometry gridLayout, width, height, size float32) {
	ui.Box(c.Key(frame.scene.id)).Absolute().Left(frame.offset*width).Top(0).Size(width, height).PassThrough().Role(ui.RoleNone).Children(func() {
		if frame.scene.request.emptyKey != "" {
			empty := ui.Text(c, v.t(frame.scene.request.emptyKey)).Absolute().Top(height/2 - 12).Left(0).Width(width).TextAlign(ui.Center).FontSize(18).TextColor(ui.RGBA(255, 255, 255, .85))
			if !frame.target {
				empty.PassThrough().Role(ui.RoleNone)
			}
		}
		for index, app := range frame.scene.request.apps {
			x, y := geometry.X+float32(index%geometry.Columns)*geometry.CellW, geometry.Y+float32(index/geometry.Columns)*geometry.CellH
			var tile ui.Element
			if frame.target {
				tile = ui.ButtonBase(c.Key(app.ID)).Column()
			} else {
				tile = ui.Column(c.Key(app.ID)).Center().Shrink(0).PassThrough().Role(ui.RoleNone)
			}
			tile.Absolute().Left(x).Top(y).Size(geometry.CellW, geometry.CellH).Radius(18).Padding(9, 4).Gap(9).AlignItems(ui.Center)
			background := ui.Transparent
			pose := ui.Pose{Scale: 1}
			if frame.target {
				tile.FocusBind(&v.focus, app.ID).Tooltip(app.Name).Label(app.Name)
				if tile.Hovered() || tile.Focused() || index+v.page*v.pageSize == v.selected {
					background = ui.RGBA(255, 255, 255, .10)
				}
				if tile.Hovered() {
					pose = ui.Pose{Y: -2, Scale: 1.025}
				}
				tile.Background(tile.AnimateColor("hover", background, ui.Animation{Duration: 140 * time.Millisecond})).AnimatePose(pose, ui.Animation{Duration: 160 * time.Millisecond, Ease: ui.EaseOut})
				tile.OnClick(func() { v.launch(app) })
			}
			tile.Children(func() {
				if bitmap := v.bitmap(app.ID); bitmap != nil {
					image := ui.Image(c, bitmap).Size(size, size).Fit(ui.Contain)
					if !frame.target {
						image.PassThrough().Role(ui.RoleNone)
					}
				} else {
					canvas := ui.Box(c).Size(size, size).Center()
					if !frame.target {
						canvas.PassThrough().Role(ui.RoleNone)
					}
					canvas.Children(func() {
						visibleSize := size - 2*iconOpticalInset(size)
						plate := ui.Box(c).Size(visibleSize, visibleSize).Radius(visibleSize * .23).Background(ui.RGBA(255, 255, 255, .13)).Center()
						if !frame.target {
							plate.PassThrough().Role(ui.RoleNone)
						}
						plate.Children(func() {
							glyph := ui.Icon(c, placeholderIcon).Size(size*.42, size*.42).TextColor(ui.RGBA(255, 255, 255, .5))
							if !frame.target {
								glyph.PassThrough().Role(ui.RoleNone)
							}
						})
					})
				}
				name := ui.Text(c, app.Name).Width(geometry.CellW - 12).SingleLine().Ellipsis("…").TextAlign(ui.Center).FontSize(13).TextColor(ui.RGB(255, 255, 255))
				if !frame.target {
					name.PassThrough().Role(ui.RoleNone)
				}
			})
		}
	})
}

func svg(content string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + content + `</svg>`))
}

var searchIcon = svg(`<circle cx="10.8" cy="10.8" r="7"/><path d="m16 16 4.5 4.5"/>`)
var closeIcon = svg(`<path d="m6 6 12 12M18 6 6 18"/>`)
var leftIcon = svg(`<path d="m14 6-6 6 6 6"/>`)
var rightIcon = svg(`<path d="m10 6 6 6-6 6"/>`)
var settingsIcon = svg(`<path d="m9.5 3-.6 2.4-2 .9-2.2-.7L2.2 10l1.7 1.7v2.2L2.2 15.5l2.5 4.3 2.3-.6 1.9 1 .6 2.3h5l.6-2.4 2-.9 2.2.7 2.5-4.4-1.7-1.6v-2.2l1.7-1.7-2.5-4.3-2.3.6-1.9-1L14.5 3Z" transform="translate(0 -1) scale(1 .96)"/><circle cx="12" cy="12" r="3"/>`)
var placeholderIcon = svg(`<rect x="3" y="3" width="7" height="7" rx="2"/><rect x="14" y="3" width="7" height="7" rx="2"/><rect x="3" y="14" width="7" height="7" rx="2"/><rect x="14" y="14" width="7" height="7" rx="2"/>`)
