package launchview

import "log"

// categorySaver is optional so lightweight catalogs used by previews and tests
// can still provide the complete launcher without a preferences store.
type categorySaver interface{ SetLastCategory(string) error }

func (v *Launchpad) rememberCategory(id string) {
	v.mu.Lock()
	v.lastCategoryIntent = id
	v.lastCategoryRevision++
	v.state.Preferences.LastCategory = id
	v.categorySaveError = ""
	if v.saveLastCategory == nil {
		v.lastCategoryProcessed = v.lastCategoryRevision
	}
	v.mu.Unlock()
	select {
	case v.saveWake <- struct{}{}:
	default:
	}
}

func (v *Launchpad) saveCategories() {
	defer close(v.saveDone)
	for {
		select {
		case <-v.stop:
			v.flushCategory()
			v.flushPages()
			return
		case <-v.saveWake:
			v.flushCategory()
			v.flushPages()
		}
	}
}

// flushCategory is the single writer. A slow or failed older write cannot
// replace a newer selection or suppress its following save. It only calls the
// store; Close can wait for it without waiting on the native UI thread.
func (v *Launchpad) flushCategory() {
	for {
		v.mu.Lock()
		revision, id := v.lastCategoryRevision, v.lastCategoryIntent
		if revision <= v.lastCategoryProcessed {
			v.mu.Unlock()
			return
		}
		v.mu.Unlock()
		var err error
		if v.saveLastCategory != nil {
			err = v.saveLastCategory(id)
		}
		v.mu.Lock()
		currentFailure := err != nil && revision == v.lastCategoryRevision
		v.lastCategoryProcessed = revision
		if revision == v.lastCategoryRevision {
			if err != nil {
				v.categorySaveError = err.Error()
			} else {
				v.categorySaveError = ""
				v.state.Preferences.LastCategory = id
			}
		}
		v.mu.Unlock()
		if currentFailure {
			log.Printf("gopad: couldn't remember category: %v", err)
		}
		select {
		case <-v.stop:
		default:
			v.invalidate()
		}
	}
}

func (v *Launchpad) retryCategorySave() { v.rememberCategory(v.category) }
func (v *Launchpad) categorySaveFailed() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.categorySaveError != ""
}
