package launcher

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

func bundleIdentity(bundleID string) string {
	sum := sha256.Sum256([]byte(bundleID))
	return "bundle:" + hex.EncodeToString(sum[:12])
}

// installAppsLocked gives the preferred copy of an application a bundle-based
// identity. Other installed copies retain separate path identities; they never
// collapse into a single tile or inherit one another's preferences.
func (s *Service) installAppsLocked(input []App) error {
	apps := append([]App(nil), input...)
	groups := make(map[string][]int)
	for i, app := range apps {
		if app.BundleID != "" {
			groups[app.BundleID] = append(groups[app.BundleID], i)
		}
	}
	previous := make(map[string]App, len(s.apps))
	for _, app := range s.apps {
		previous[app.ID] = app
	}
	remap := make(map[string]string)
	p := clonePreferences(s.preferences)
	changed := false
	for bundleID, indices := range groups {
		stable := bundleIdentity(bundleID)
		oldPrimary := previous[stable].Path
		if oldPrimary == "" {
			oldPrimary = p.AppLocations[stable]
		}
		sort.SliceStable(indices, func(i, j int) bool {
			a, b := apps[indices[i]].Path, apps[indices[j]].Path
			if (a == oldPrimary) != (b == oldPrimary) {
				return a == oldPrimary
			}
			// Prefer the ordinary Applications copy over a nested backup.
			priority := func(path string) int {
				if strings.HasPrefix(path, "/Applications/") && strings.Count(strings.TrimPrefix(path, "/Applications/"), "/") == 0 {
					return 0
				}
				if strings.HasPrefix(path, "/System/") {
					return 1
				}
				return 2
			}
			if priority(a) != priority(b) {
				return priority(a) < priority(b)
			}
			return a < b
		})
		for position, index := range indices {
			old := apps[index].ID
			id := applicationID(apps[index].Path)
			if position == 0 {
				id = stable
				if p.AppLocations[id] != apps[index].Path {
					p.AppLocations[id] = apps[index].Path
					changed = true
				}
			}
			apps[index].ID = id
			if old != "" && old != id {
				remap[old] = id
			}
			legacy := applicationID(apps[index].Path)
			if legacy != id {
				remap[legacy] = id
			}
		}
	}
	mapIDs := func(ids []string) []string {
		for i, id := range ids {
			if replacement, ok := remap[id]; ok && replacement != id {
				ids[i] = replacement
				changed = true
			}
		}
		return ids
	}
	p.Hidden = mapIDs(p.Hidden)
	p.Favorites = mapIDs(p.Favorites)
	p.Recent = mapIDs(p.Recent)
	p.Order = mapIDs(p.Order)
	for i := range p.Categories {
		p.Categories[i].AppIDs = mapIDs(p.Categories[i].AppIDs)
	}
	for old, next := range remap {
		if alias, ok := p.Aliases[old]; ok {
			if _, exists := p.Aliases[next]; !exists {
				p.Aliases[next] = alias
			}
			delete(p.Aliases, old)
			changed = true
		}
	}
	if changed {
		// Persist migration before publishing new IDs, so an I/O failure cannot
		// leave the UI and the preference file referring to different apps.
		if _, err := s.savePreferencesLocked(p); err != nil {
			return err
		}
	}
	validIcons := make(map[string]bool, len(apps))
	for _, app := range apps {
		validIcons[iconCacheKey(app)] = true
	}
	s.iconMu.Lock()
	for key := range s.icons {
		if !validIcons[key] {
			delete(s.icons, key)
		}
	}
	s.iconMu.Unlock()
	s.apps, s.loaded, s.lastScan = apps, true, time.Now()
	return nil
}

func iconCacheKey(app App) string {
	return app.ID + "\x00" + app.Path + "\x00" + app.BundleID + "\x00" + app.Version
}
