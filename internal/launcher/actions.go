package launcher

import (
	"errors"
	"slices"
)

// SetPage changes only navigation memory, preserving simultaneous app edits.
func (s *Service) SetPage(category string, page int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := clonePreferences(s.preferences)
	if !(category == "recent" && p.ShowRecentApps) && !slices.ContainsFunc(p.Categories, func(c Category) bool { return c.ID == category }) {
		return errors.New("application section no longer exists")
	}
	page = max(0, min(page, maxAppIDs))
	if p.PageMemory[category] == page {
		return nil
	}
	p.PageMemory[category] = page
	_, err := s.savePreferencesLocked(p)
	return err
}

// ApplyAppAction mutates the latest preferences under one lock. The launcher
// can make a quick edit without overwriting an unrelated settings-window edit.
func (s *Service) ApplyAppAction(action, id, target string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return State{}, err
	}
	if !slices.ContainsFunc(s.apps, func(app App) bool { return app.ID == id }) {
		return State{}, errors.New("application no longer exists; refresh the application list")
	}
	p := clonePreferences(s.preferences)
	remove := func(ids []string, id string) []string {
		return slices.DeleteFunc(ids, func(value string) bool { return value == id })
	}
	switch action {
	case "favorite-add":
		if !slices.Contains(p.Favorites, id) {
			p.Favorites = append(p.Favorites, id)
		}
	case "favorite":
		if slices.Contains(p.Favorites, id) {
			p.Favorites = remove(p.Favorites, id)
		} else {
			p.Favorites = append(p.Favorites, id)
		}
	case "hide":
		if !slices.Contains(p.Hidden, id) {
			p.Hidden = append(p.Hidden, id)
		}
	case "restore":
		p.Hidden = remove(p.Hidden, id)
	case "category", "remove-category":
		index := slices.IndexFunc(p.Categories, func(c Category) bool { return c.ID == target })
		if index < 0 || target == "all" || target == "favorites" || target == "system" || target == "hidden" {
			return State{}, errors.New("custom application section no longer exists")
		}
		if action == "remove-category" {
			p.Categories[index].AppIDs = remove(p.Categories[index].AppIDs, id)
		} else if !slices.Contains(p.Categories[index].AppIDs, id) {
			p.Categories[index].AppIDs = append(p.Categories[index].AppIDs, id)
		}
	case "reorder":
		if target == id {
			return s.stateLocked(), nil
		}
		if !slices.ContainsFunc(s.apps, func(app App) bool { return app.ID == target }) {
			return State{}, errors.New("destination application no longer exists")
		}
		// Materialize the full current order first. An empty manual order must
		// still preserve all the surrounding tiles when a single app moves.
		order := append([]string(nil), p.Order...)
		seen := make(map[string]bool, len(order)+len(s.apps))
		for _, appID := range order {
			seen[appID] = true
		}
		for _, app := range s.apps {
			if !seen[app.ID] {
				order = append(order, app.ID)
				seen[app.ID] = true
			}
		}
		order = remove(order, id)
		index := slices.Index(order, target)
		p.Order = slices.Insert(order, index, id)
	default:
		return State{}, errors.New("unknown application action")
	}
	return s.savePreferencesLocked(p)
}
