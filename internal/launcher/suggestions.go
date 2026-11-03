package launcher

import (
	"log"
	"slices"
	"strings"

	"github.com/knothhe/gopad/internal/i18n"
)

// recordLaunch records only opted-in, successfully launched apps. History is
// best effort: an unavailable preference disk must never turn a successful
// application launch into a user-visible launch failure.
func (s *Service) recordLaunch(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.preferences.ShowRecentApps {
		return
	}
	p := clonePreferences(s.preferences)
	p.Recent = slices.DeleteFunc(p.Recent, func(appID string) bool { return appID == id })
	p.Recent = append([]string{id}, p.Recent...)
	p.Recent = p.Recent[:min(len(p.Recent), 24)]
	if slices.Equal(p.Recent, s.preferences.Recent) {
		return
	}
	if _, err := s.savePreferencesLocked(p); err != nil {
		log.Printf("gopad: could not save recently launched apps: %v", err)
	}
}

// SuggestedCategories creates editable custom sections from declared bundle
// metadata. It never guesses an application's purpose from its display name.
func SuggestedCategories(state State) []Category {
	return SuggestedCategoriesForLocale(state, i18n.Resolve(state.Preferences.Language, i18n.EnvironmentLocale()))
}

// SuggestedCategoriesForLocale lets the desktop shell supply the effective
// system locale, which need not be present in a GUI application's environment.
func SuggestedCategoriesForLocale(state State, locale string) []Category {
	groups := map[string][]string{}
	seen := map[string]bool{}
	for _, app := range state.Apps {
		if app.ID == "" || seen[app.ID] || app.Kind == "system" || strings.HasPrefix(app.Path, "/System/") || slices.Contains(state.Preferences.Hidden, app.ID) {
			continue
		}
		group := suggestedGroup(app.Category)
		if group == "" {
			continue
		}
		seen[app.ID] = true
		groups[group] = append(groups[group], app.ID)
	}
	existing := map[string]bool{}
	for _, category := range state.Preferences.Categories {
		existing[category.ID] = true
	}
	result := []Category{}
	for _, group := range []string{"development", "productivity", "creativity", "communication", "entertainment", "utilities", "education"} {
		id := "suggested-" + group
		if existing[id] || len(groups[group]) < 2 {
			continue
		}
		result = append(result, Category{ID: id, Name: i18n.T(locale, "category.suggested."+group), AppIDs: groups[group], Visible: true})
	}
	return result
}

func suggestedGroup(metadata string) string {
	const prefix = "public.app-category."
	if !strings.HasPrefix(metadata, prefix) {
		return ""
	}
	category := strings.TrimPrefix(metadata, prefix)
	switch category {
	case "developer-tools":
		return "development"
	case "productivity", "business", "finance":
		return "productivity"
	case "graphics-design", "photography", "video", "music":
		return "creativity"
	case "social-networking":
		return "communication"
	case "entertainment", "games", "sports":
		return "entertainment"
	case "utilities":
		return "utilities"
	case "education", "reference":
		return "education"
	default:
		// App Store game subcategories also declare the public namespace.
		if strings.HasSuffix(category, "-games") {
			return "entertainment"
		}
		return ""
	}
}

// AddSuggestedCategories preserves every existing section, including earlier
// suggestions that the user has since renamed or edited.
func (s *Service) AddSuggestedCategories() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadLocked(); err != nil {
		return State{}, err
	}
	p := clonePreferences(s.preferences)
	suggestions := SuggestedCategories(s.stateLocked())
	if len(suggestions) == 0 {
		return s.stateLocked(), nil
	}
	p.Categories = append(p.Categories, suggestions...)
	return s.savePreferencesLocked(p)
}
