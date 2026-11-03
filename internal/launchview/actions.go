package launchview

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/launcher"
	"github.com/knothhe/gopad/internal/uikit"
)

type appActionCatalog interface {
	ApplyAppAction(action, id, target string) (launcher.State, error)
}

type appRevealer interface{ Reveal(string) error }

// appDrag is deliberately private: a drop accepts only an app dragged from
// this launcher, never arbitrary text or another application's payload.
type appDrag struct{ ID string }

type appActionState struct {
	pending  bool
	undoID   string
	undoName string
	queue    []queuedAppAction
	done     chan struct{}
}

type queuedAppAction struct{ action, id, target string }

const maxQueuedAppActions = 64

func (v *Launchpad) actionPending() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.actions.pending
}

// applyAppAction uses a fresh backend transaction rather than saving the
// launcher's snapshot, so simultaneous settings edits are preserved.
func (v *Launchpad) applyAppAction(action, id, target string) {
	store, ok := v.catalog.(appActionCatalog)
	if !ok {
		return
	}
	v.mu.Lock()
	select {
	case <-v.stop:
		v.mu.Unlock()
		return
	default:
	}
	if v.actions.pending {
		if len(v.actions.queue) >= maxQueuedAppActions {
			v.errKey, v.errDetail = "native.error.actionQueue", ""
			v.mu.Unlock()
			v.invalidate()
			return
		}
		v.actions.queue = append(v.actions.queue, queuedAppAction{action, id, target})
		v.mu.Unlock()
		return
	}
	v.actions.pending = true
	v.actions.done = make(chan struct{})
	v.generation++
	v.mu.Unlock()
	go func() {
		for {
			v.mu.Lock()
			name := id
			for _, app := range v.state.Apps {
				if app.ID == id {
					name = app.Name
					break
				}
			}
			v.mu.Unlock()
			state, err := store.ApplyAppAction(action, id, target)
			v.mu.Lock()
			// Any older scan must not replace this mutation's preferences.
			v.generation++
			if err != nil {
				v.errKey, v.errDetail = "native.error.action", err.Error()
			} else {
				if v.lastCategoryIntent != "" {
					state.Preferences.LastCategory = v.lastCategoryIntent
				}
				v.state, v.loaded = state, true
				v.errKey, v.errDetail = "", ""
				if action == "hide" {
					v.actions.undoID, v.actions.undoName = id, name
				} else if action == "restore" && v.actions.undoID == id {
					v.actions.undoID, v.actions.undoName = "", ""
				}
			}
			more := len(v.actions.queue) > 0
			if more {
				next := v.actions.queue[0]
				v.actions.queue = v.actions.queue[1:]
				action, id, target = next.action, next.id, next.target
			} else {
				v.actions.pending = false
				v.actions.queue = nil
				close(v.actions.done)
			}
			v.mu.Unlock()
			select {
			case <-v.stop:
			default:
				v.invalidate()
			}
			if !more {
				return
			}
		}
	}()
}

// Close calls this after closing stop. Backend saves are independent of the UI
// thread, so already queued edits can finish without reopening a closed view.
func (v *Launchpad) waitAppActions() {
	v.mu.Lock()
	done := v.actions.done
	v.mu.Unlock()
	if done != nil {
		<-done
	}
}

func (v *Launchpad) undoHidden() {
	v.mu.Lock()
	id := v.actions.undoID
	v.mu.Unlock()
	if id != "" {
		v.applyAppAction("restore", id, "")
	}
}

func (v *Launchpad) hasHiddenUndo() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.actions.undoID != ""
}

func (v *Launchpad) revealApp(id string) {
	revealer, ok := v.catalog.(appRevealer)
	if !ok {
		return
	}
	go func() {
		err := revealer.Reveal(id)
		if err != nil {
			v.mu.Lock()
			v.errKey, v.errDetail = "native.error.action", err.Error()
			v.mu.Unlock()
			v.invalidate()
		} else if v.callbacks.Hide != nil {
			v.callbacks.Hide()
		}
	}()
}

// bindAppActions leaves click handling to the tile. MyGo starts the typed drag
// only after its movement threshold and suppresses the source's click on drop.
func (v *Launchpad) bindAppActions(tile ui.Element, app launcher.App) {
	if v.bindFolderActions(tile, app) {
		return
	}
	_, editable := v.catalog.(appActionCatalog)
	if editable {
		tile.Drag(appDrag{ID: app.ID})
		if source, over := ui.DragOver[appDrag](tile); over && source.ID != app.ID && v.query == "" {
			tile.Border(2, ui.RGBA(255, 255, 255, .65))
		}
		if source, dropped := ui.Drop[appDrag](tile); dropped && source.ID != app.ID && v.query == "" {
			v.applyAppAction("reorder", source.ID, app.ID)
		}
	}
	tile.ContextMenu(func(menu *ui.Menu) {
		state, _, _ := v.snapshot()
		pending := v.actionPending()
		if menu.Item(v.t("native.menu.open")).Shortcut(0, ui.KeyEnter).Chosen() {
			v.launch(app)
		}
		if editable {
			favorite := slices.Contains(state.Preferences.Favorites, app.ID)
			key := "native.menu.favorite"
			if favorite {
				key = "native.menu.unfavorite"
			}
			if menu.Item(v.t(key)).Disabled(pending).Chosen() {
				v.applyAppAction("favorite", app.ID, "")
			}
			categories := launcher.CategoriesFor(state.Preferences)
			custom := slices.DeleteFunc(categories, func(category launcher.Category) bool {
				return !customCategory(category.ID)
			})
			if len(custom) > 0 {
				menu.Submenu(v.t("native.menu.categories"), func(sub *ui.Menu) {
					for _, category := range custom {
						member := slices.Contains(category.AppIDs, app.ID)
						if sub.Item(category.Name).Checked(member).Disabled(pending).Chosen() {
							action := "category"
							if member {
								action = "remove-category"
							}
							v.applyAppAction(action, app.ID, category.ID)
						}
					}
				})
			}
		}
		if _, ok := v.catalog.(appRevealer); ok {
			menu.Separator()
			if menu.Item(v.t("native.menu.reveal")).Chosen() {
				v.revealApp(app.ID)
			}
		}
		if editable {
			menu.Separator()
			hidden := slices.Contains(state.Preferences.Hidden, app.ID)
			key, action := "native.menu.hide", "hide"
			if hidden {
				key, action = "native.menu.restore", "restore"
			}
			if menu.Item(v.t(key)).Disabled(pending).Chosen() {
				v.applyAppAction(action, app.ID, "")
			}
		}
	})
}

// categoryDropTarget is the optional tab decorator passed to uikit.Tabs.
func (v *Launchpad) categoryDropTarget(tab ui.Element, categoryID string) {
	if _, ok := v.catalog.(appActionCatalog); !ok || categoryID == "all" || categoryID == "system" || categoryID == "hidden" || categoryID == "recent" {
		return
	}
	if _, over := ui.DragOver[appDrag](tab); over {
		tab.Border(2, ui.RGBA(255, 255, 255, .7))
	}
	if app, dropped := ui.Drop[appDrag](tab); dropped {
		action := "category"
		if categoryID == "favorites" {
			state, _, _ := v.snapshot()
			if slices.Contains(state.Preferences.Favorites, app.ID) {
				return
			}
			action = "favorite-add"
		}
		v.applyAppAction(action, app.ID, categoryID)
	}
}

func (v *Launchpad) renderActionFeedback(c *ui.Context, width, height float32) {
	v.mu.Lock()
	id, name := v.actions.undoID, v.actions.undoName
	errorVisible := v.errKey != "" || v.categorySaveError != ""
	v.mu.Unlock()
	if id == "" || errorVisible {
		return
	}
	uikit.Surface(c.Key("app-action-feedback"), 12, uikit.Regular).Absolute().Bottom(78).Left(max(16, (width-360)/2)).Width(min(width-32, 360)).Padding(10, 14).Gap(12).AlignItems(ui.Center).OnClick(func() {}).Children(func() {
		ui.Text(c, v.t("native.hidden", "name", name)).Grow(1).SingleLine().Ellipsis("…").FontSize(13)
		v.renderUndoButton(c)
	}).AnimatePose(ui.Pose{Scale: 1}, ui.Animation{Duration: 120 * time.Millisecond})
}

// renderUndoButton can share the launcher's error surface, keeping a previous
// successful hide reversible without two feedback surfaces painting over it.
func (v *Launchpad) renderUndoButton(c *ui.Context) {
	v.mu.Lock()
	id, pending := v.actions.undoID, v.actions.pending
	v.mu.Unlock()
	if id == "" {
		return
	}
	ui.ButtonBase(c.Key("undo-hide")).Padding(5, 9).Radius(7).Background(ui.RGBA(255, 255, 255, .12)).Label(v.t("native.undo")).Disabled(pending).Children(func() {
		ui.Text(c, v.t("native.undo")).FontSize(13)
	}).OnClick(v.undoHidden)
}
