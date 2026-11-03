package launcher

import (
	"github.com/knothhe/gopad/internal/i18n"
	"slices"
	"strings"
	"unicode"
)

const (
	maxCustomCategories  = 64
	maxCategoryIDLength  = 80
	maxCategoryNameRunes = 40
	maxAppIDs            = 10000
	maxAppIDLength       = 128
)

// Category is a launchpad section. Its position in Preferences.Categories is
// its display order. Built-in membership is derived from application metadata
// and preferences; a custom section contains its AppIDs, even while those apps
// are temporarily uninstalled.
type Category struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	AppIDs  []string `json:"appIds"`
	Visible bool     `json:"visible"`
}

// DefaultCategories returns fresh values; callers may edit them independently.
func DefaultCategories() []Category {
	return []Category{
		{ID: "all", Name: i18n.T("zh-CN", "category.all"), AppIDs: []string{}, Visible: true},
		{ID: "favorites", Name: i18n.T("zh-CN", "category.favorites"), AppIDs: []string{}, Visible: true},
		{ID: "system", Name: i18n.T("zh-CN", "category.system"), AppIDs: []string{}, Visible: true},
		{ID: "hidden", Name: i18n.T("zh-CN", "category.hidden"), AppIDs: []string{}, Visible: false},
	}
}

// CategoriesFor migrates older preferences, restores omitted built-in sections,
// and returns a normalized deep copy without changing p. Existing order and
// visibility win over defaults; only custom sections can be removed.
func CategoriesFor(p Preferences) []Category { return normalizeCategories(p.Categories) }

// VisibleCategories preserves the configured section order and returns an
// independent copy containing only sections shown on the launchpad.
func VisibleCategories(p Preferences) []Category {
	all := CategoriesFor(p)
	visible := make([]Category, 0, len(all))
	for _, category := range all {
		if category.Visible {
			visible = append(visible, category)
		}
	}
	if p.ShowRecentApps {
		visible = append(visible, Category{ID: "recent", Name: i18n.T(i18n.Resolve(p.Language, i18n.EnvironmentLocale()), "category.recent"), AppIDs: []string{}, Visible: true})
	}
	return visible
}

// CategoryContains reports membership independently of a section's visibility.
// Hidden apps belong exclusively to the hidden section; custom and built-in
// sections otherwise follow their own membership rules.
func CategoryContains(category Category, app App, p Preferences) bool {
	if app.ID == "" {
		return false
	}
	hidden := slices.Contains(p.Hidden, app.ID)
	if category.ID == "hidden" {
		return hidden
	}
	if hidden {
		return false
	}
	switch category.ID {
	case "all":
		return true
	case "favorites":
		return slices.Contains(p.Favorites, app.ID)
	case "recent":
		return slices.Contains(p.Recent, app.ID)
	case "system":
		return app.Kind == "system" || strings.HasPrefix(app.Path, "/System/")
	default:
		return slices.Contains(category.AppIDs, app.ID)
	}
}

func normalizeCategories(input []Category) []Category {
	defaults := DefaultCategories()
	builtins := make(map[string]Category, len(defaults))
	for _, category := range defaults {
		builtins[category.ID] = category
	}
	result := make([]Category, 0, min(len(input)+len(defaults), maxCustomCategories+len(defaults)))
	seen := make(map[string]bool)
	customCount := 0
	for _, category := range input {
		category.ID = strings.TrimSpace(category.ID)
		if !validCategoryID(category.ID) || seen[category.ID] || category.ID == "recent" {
			continue
		}
		if builtin, ok := builtins[category.ID]; ok {
			category.Name = builtin.Name
			category.AppIDs = []string{}
		} else {
			if customCount >= maxCustomCategories {
				continue
			}
			category.Name = cleanCategoryName(category.Name)
			if category.Name == "" {
				continue
			}
			category.AppIDs = uniqueIDs(category.AppIDs)
			customCount++
		}
		seen[category.ID] = true
		result = append(result, category)
	}
	for _, category := range defaults {
		if !seen[category.ID] {
			result = append(result, category)
		}
	}
	if !slices.ContainsFunc(result, func(category Category) bool { return category.Visible }) {
		for i := range result {
			if result[i].ID == "all" {
				result[i].Visible = true
				break
			}
		}
	}
	return result
}

func cloneCategories(input []Category) []Category {
	result := make([]Category, len(input))
	for i, category := range input {
		category.AppIDs = append([]string{}, category.AppIDs...)
		result[i] = category
	}
	return result
}

func validCategoryID(id string) bool {
	if len(id) == 0 || len(id) > maxCategoryIDLength {
		return false
	}
	for _, r := range id {
		if !asciiAlphanumeric(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func validAppID(id string) bool {
	if len(id) == 0 || len(id) > maxAppIDLength {
		return false
	}
	for _, r := range id {
		if !asciiAlphanumeric(r) && r != '-' && r != '_' && r != '.' && r != ':' {
			return false
		}
	}
	return true
}

func asciiAlphanumeric(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func cleanCategoryName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.ToValidUTF8(name, ""))
	runes := []rune(strings.TrimSpace(name))
	if len(runes) > maxCategoryNameRunes {
		runes = runes[:maxCategoryNameRunes]
	}
	return strings.TrimSpace(string(runes))
}
