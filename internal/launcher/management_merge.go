package launcher

import "slices"

// SaveManagementPreferencesWithBase applies the user's changes to the latest
// state. A settings window may stay open while the native launcher changes app
// organization; untouched fields from that older window must not replace them.
func (s *Service) SaveManagementPreferencesWithBase(next, base Preferences) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return State{}, err
	}
	return s.savePreferencesLocked(MergeManagementPreferences(s.preferences, next, base))
}

// MergeManagementPreferences is also used before preparing system settings, so
// the shortcut/login changes being validated match the values being persisted.
// Runtime navigation and discovered application identities always remain live.
func MergeManagementPreferences(current, next, base Preferences) Preferences {
	current = normalizePreferences(clonePreferences(current))
	next = normalizePreferences(clonePreferences(next))
	base = normalizePreferences(clonePreferences(base))
	p := clonePreferences(current)
	p.IconSize = mergePreferenceValue(current.IconSize, next.IconSize, base.IconSize)
	p.Language = mergePreferenceValue(current.Language, next.Language, base.Language)
	p.ShowMenuBarIcon = mergePreferenceValue(current.ShowMenuBarIcon, next.ShowMenuBarIcon, base.ShowMenuBarIcon)
	p.SearchScope = mergePreferenceValue(current.SearchScope, next.SearchScope, base.SearchScope)
	p.SearchRanking = mergePreferenceValue(current.SearchRanking, next.SearchRanking, base.SearchRanking)
	p.LoopPages = mergePreferenceValue(current.LoopPages, next.LoopPages, base.LoopPages)
	p.RememberPage = mergePreferenceValue(current.RememberPage, next.RememberPage, base.RememberPage)
	p.DismissOnEmptyClick = mergePreferenceValue(current.DismissOnEmptyClick, next.DismissOnEmptyClick, base.DismissOnEmptyClick)
	p.EscapeClearsSearch = mergePreferenceValue(current.EscapeClearsSearch, next.EscapeClearsSearch, base.EscapeClearsSearch)
	p.RememberCategory = mergePreferenceValue(current.RememberCategory, next.RememberCategory, base.RememberCategory)
	p.TabCyclesCategories = mergePreferenceValue(current.TabCyclesCategories, next.TabCyclesCategories, base.TabCyclesCategories)
	p.Shortcut = mergePreferenceValue(current.Shortcut, next.Shortcut, base.Shortcut)
	p.LaunchAtLogin = mergePreferenceValue(current.LaunchAtLogin, next.LaunchAtLogin, base.LaunchAtLogin)
	p.ShowRecentApps = mergePreferenceValue(current.ShowRecentApps, next.ShowRecentApps, base.ShowRecentApps)
	p.ShowCategoryFolders = mergePreferenceValue(current.ShowCategoryFolders, next.ShowCategoryFolders, base.ShowCategoryFolders)
	p.Hidden = mergePreferenceSet(current.Hidden, next.Hidden, base.Hidden)
	p.Favorites = mergePreferenceSet(current.Favorites, next.Favorites, base.Favorites)
	p.Order = mergePreferenceOrder(current.Order, next.Order, base.Order)
	p.Categories = mergePreferenceCategories(current.Categories, next.Categories, base.Categories)
	for key, old := range base.Aliases {
		value, exists := next.Aliases[key]
		if !exists {
			delete(p.Aliases, key)
		} else if value != old {
			p.Aliases[key] = value
		}
	}
	for key, value := range next.Aliases {
		if _, existed := base.Aliases[key]; !existed {
			p.Aliases[key] = value
		}
	}
	return normalizePreferences(p)
}

func mergePreferenceValue[T comparable](current, next, base T) T {
	if next != base {
		return next
	}
	return current
}

func mergePreferenceSet(current, next, base []string) []string {
	before := make(map[string]bool, len(base))
	after := make(map[string]bool, len(next))
	for _, id := range base {
		before[id] = true
	}
	for _, id := range next {
		after[id] = true
	}
	result := make([]string, 0, len(current)+len(next))
	seen := make(map[string]bool)
	for _, id := range current {
		if before[id] && !after[id] {
			continue
		}
		if !seen[id] {
			result = append(result, id)
			seen[id] = true
		}
	}
	for _, id := range next {
		if !before[id] && !seen[id] {
			result = append(result, id)
			seen[id] = true
		}
	}
	return result
}

func mergePreferenceOrder(current, next, base []string) []string {
	if slices.Equal(next, base) {
		return append([]string{}, current...)
	}
	result := append([]string{}, next...)
	before := make(map[string]bool, len(base))
	seen := make(map[string]bool, len(next))
	for _, id := range base {
		before[id] = true
	}
	for _, id := range next {
		seen[id] = true
	}
	for _, id := range current {
		if !before[id] && !seen[id] {
			result = append(result, id)
			seen[id] = true
		}
	}
	return result
}

func mergePreferenceCategories(current, next, base []Category) []Category {
	before := make(map[string]Category, len(base))
	after := make(map[string]Category, len(next))
	live := make(map[string]Category, len(current))
	ids := func(categories []Category) []string {
		result := make([]string, len(categories))
		for i, c := range categories {
			result[i] = c.ID
		}
		return result
	}
	for _, c := range base {
		before[c.ID] = c
	}
	for _, c := range next {
		after[c.ID] = c
	}
	for _, c := range current {
		live[c.ID] = c
	}
	for id := range before {
		if _, exists := after[id]; !exists {
			delete(live, id)
		}
	}
	for _, c := range next {
		old, existed := before[c.ID]
		latest, exists := live[c.ID]
		if !existed {
			if exists {
				c.AppIDs = mergePreferenceSet(latest.AppIDs, c.AppIDs, nil)
			}
			live[c.ID] = c
			continue
		}
		// Keep a concurrently deleted category deleted rather than recreating
		// it through an edit from a window that still displays the old row.
		if !exists {
			continue
		}
		latest.Name = mergePreferenceValue(latest.Name, c.Name, old.Name)
		latest.Visible = mergePreferenceValue(latest.Visible, c.Visible, old.Visible)
		latest.AppIDs = mergePreferenceSet(latest.AppIDs, c.AppIDs, old.AppIDs)
		live[c.ID] = latest
	}
	order := mergePreferenceOrder(ids(current), ids(next), ids(base))
	result := make([]Category, 0, len(live))
	seen := make(map[string]bool, len(live))
	for _, id := range order {
		if c, ok := live[id]; ok && !seen[id] {
			result = append(result, c)
			seen[id] = true
		}
	}
	return result
}
