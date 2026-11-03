package launchview

import "github.com/knothhe/gopad/internal/launcher"

type launchBehavior struct {
	wallpaperData []byte
	pages         map[string]int
	pagesRestored bool
	pendingPages  map[string]int // protected by Launchpad.mu, drained by saveCategories
	searching     bool
	returnPage    int
	browsePage    int
}

func (v *Launchpad) searchCategory(p launcher.Preferences) launcher.Category {
	category := v.currentCategory()
	if v.query != "" && p.SearchScope != "current" && category.ID != "hidden" {
		return launcher.Category{ID: "all"}
	}
	return category
}

func (v *Launchpad) restorePages(state launcher.State, loaded bool) {
	if !loaded || v.behavior.pagesRestored {
		return
	}
	v.behavior.pagesRestored = true
	v.behavior.pages = make(map[string]int)
	for id, page := range state.Preferences.PageMemory {
		v.behavior.pages[id] = page
	}
	if state.Preferences.RememberPage && v.page == 0 && v.query == "" {
		v.page = v.behavior.pages[v.category]
	}
}

func (v *Launchpad) savedPage(category string) int {
	state, _, _ := v.snapshot()
	if !state.Preferences.RememberPage {
		return 0
	}
	if v.behavior.pages != nil {
		return v.behavior.pages[category]
	}
	return state.Preferences.PageMemory[category]
}

func (v *Launchpad) rememberCurrentPage() {
	if v.query != "" || v.behavior.searching || v.category == "" {
		return
	}
	v.mu.Lock()
	if !v.state.Preferences.RememberPage {
		v.mu.Unlock()
		return
	}
	if v.behavior.pages == nil {
		v.behavior.pages = make(map[string]int)
	}
	if old, exists := v.behavior.pages[v.category]; exists && old == v.page {
		v.mu.Unlock()
		return
	}
	v.behavior.pages[v.category] = v.page
	if v.behavior.pendingPages == nil {
		v.behavior.pendingPages = make(map[string]int)
	}
	v.behavior.pendingPages[v.category] = v.page
	v.mu.Unlock()
	if v.saveWake != nil {
		select {
		case v.saveWake <- struct{}{}:
		default:
		}
	}
}

func (v *Launchpad) flushPages() {
	v.mu.Lock()
	pending := v.behavior.pendingPages
	v.behavior.pendingPages = nil
	v.mu.Unlock()
	store, ok := v.catalog.(interface{ SetPage(string, int) error })
	if !ok {
		return
	}
	failed := false
	for id, page := range pending {
		if err := store.SetPage(id, page); err != nil {
			failed = true
			v.mu.Lock()
			v.errKey, v.errDetail = "native.error.pageSave", err.Error()
			if v.behavior.pendingPages == nil {
				v.behavior.pendingPages = make(map[string]int)
			}
			if _, newer := v.behavior.pendingPages[id]; !newer {
				v.behavior.pendingPages[id] = page
			}
			v.mu.Unlock()
			v.invalidate()
		}
	}
	if !failed && len(pending) > 0 {
		v.mu.Lock()
		if v.errKey == "native.error.pageSave" {
			v.errKey, v.errDetail = "", ""
		}
		v.mu.Unlock()
		v.invalidate()
	}
}

func (v *Launchpad) pageSaveFailed() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.errKey == "native.error.pageSave"
}
func (v *Launchpad) retryPageSave() {
	select {
	case v.saveWake <- struct{}{}:
	default:
	}
}

func (v *Launchpad) queryChanged() {
	if v.query != "" {
		if !v.behavior.searching {
			v.behavior.returnPage = v.behavior.browsePage
			v.behavior.searching = true
		}
		v.page, v.selected = 0, 0
	} else {
		if v.behavior.searching {
			v.page = v.behavior.returnPage
		}
		v.behavior.searching = false
		v.selected = -1
	}
	v.focus = "search"
}

func (v *Launchpad) clearSearch() { v.query = ""; v.queryChanged() }
