package launchview

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/launcher"
)

const folderPrefix = "folder:"

func customCategory(id string) bool {
	return id != "all" && id != "favorites" && id != "system" && id != "hidden" && id != "recent"
}

func folderCategoryID(app launcher.App) (string, bool) {
	if app.Kind != "folder" || !strings.HasPrefix(app.ID, folderPrefix) {
		return "", false
	}
	id := strings.TrimPrefix(app.ID, folderPrefix)
	return id, id != "" && customCategory(id)
}

// folderApps presents existing custom categories as folders without creating a
// second grouping model. Search always returns real apps, including all members
// of folders, and membership in several folders remains valid.
func folderApps(state launcher.State, apps []launcher.App, query string, category launcher.Category) []launcher.App {
	if !state.Preferences.ShowCategoryFolders || query != "" || category.ID != "all" {
		return apps
	}
	folders := make([]launcher.App, 0)
	grouped := make(map[string]bool)
	for _, folder := range launcher.VisibleCategories(state.Preferences) {
		if !customCategory(folder.ID) {
			continue
		}
		folders = append(folders, launcher.App{ID: folderPrefix + folder.ID, Name: folder.Name, Kind: "folder"})
		for _, id := range folder.AppIDs {
			grouped[id] = true
		}
	}
	for _, app := range apps {
		if !grouped[app.ID] {
			folders = append(folders, app)
		}
	}
	return folders
}

func (v *Launchpad) openFolder(app launcher.App) bool {
	id, folder := folderCategoryID(app)
	if !folder {
		return false
	}
	v.selectCategory(id)
	return true
}

func (v *Launchpad) folderMembers(app launcher.App) []launcher.App {
	id, folder := folderCategoryID(app)
	if !folder {
		return nil
	}
	state, _, _ := v.snapshot()
	for _, category := range launcher.CategoriesFor(state.Preferences) {
		if category.ID == id {
			return appsForCategory(state, "", category)
		}
	}
	return nil
}

// renderFolderContents occupies the same canvas and optical plate as an app's
// native icon. Members use their normal cached icons at preview size.
func (v *Launchpad) renderFolderContents(c *ui.Context, app launcher.App, size float32) bool {
	if _, folder := folderCategoryID(app); !folder {
		return false
	}
	members := v.folderMembers(app)
	plateSize := size - 2*iconOpticalInset(size)
	ui.Box(c).Size(size, size).Center().PassThrough().Children(func() {
		ui.Box(c).Size(plateSize, plateSize).Radius(plateSize*.23).Background(ui.RGBA(255, 255, 255, .20)).Border(1, ui.RGBA(255, 255, 255, .20)).PassThrough().Children(func() {
			if len(members) == 0 {
				ui.Icon(c, folderGlyph).Absolute().Left(plateSize*.27).Top(plateSize*.27).Size(plateSize*.46, plateSize*.46).TextColor(ui.RGBA(255, 255, 255, .62)).PassThrough()
			}
			previewSize := plateSize * .41
			for i, member := range members[:min(4, len(members))] {
				x, y := plateSize*.07+float32(i%2)*plateSize*.45, plateSize*.07+float32(i/2)*plateSize*.45
				if bitmap := v.bitmap(member.ID); bitmap != nil {
					ui.Image(c, bitmap).Absolute().Left(x).Top(y).Size(previewSize, previewSize).Fit(ui.Contain).PassThrough()
				} else {
					inset := previewSize * iconOpticalInsetRatio
					ui.Box(c).Absolute().Left(x+inset).Top(y+inset).Size(previewSize-2*inset, previewSize-2*inset).Radius(previewSize * .18).Background(ui.RGBA(255, 255, 255, .22)).PassThrough()
				}
			}
		})
	})
	return true
}

func (v *Launchpad) bindFolderActions(tile ui.Element, app launcher.App) bool {
	id, folder := folderCategoryID(app)
	if !folder {
		return false
	}
	tile.OnClick(func() { v.openFolder(app) })
	if _, editable := v.catalog.(appActionCatalog); editable {
		if _, over := ui.DragOver[appDrag](tile); over {
			tile.Border(2, ui.RGBA(255, 255, 255, .65))
		}
		if source, dropped := ui.Drop[appDrag](tile); dropped {
			v.applyAppAction("category", source.ID, id)
		}
	}
	tile.ContextMenu(func(menu *ui.Menu) {
		if menu.Item(v.t("native.menu.open")).Chosen() {
			v.openFolder(app)
		}
		if v.callbacks.OpenManager != nil && menu.Item(v.t("native.manage")).Chosen() {
			v.callbacks.OpenManager()
		}
	})
	return true
}

// The tabs and folder view edit the same categories. A visible return affordance
// makes this relationship clear when a custom category was entered as a folder.
func (v *Launchpad) renderFolderBack(c *ui.Context, state launcher.State) {
	if !state.Preferences.ShowCategoryFolders || !customCategory(v.category) || v.query != "" {
		return
	}
	if !slices.ContainsFunc(v.categories, func(category launcher.Category) bool { return category.ID == "all" }) {
		return
	}
	width, height := c.Size()
	size := float32(state.Preferences.IconSize)
	if size < 48 || size > 112 {
		size = 80
	}
	x, y, _ := categoryGeometry(layout(width, height, size), size)
	ui.ButtonBase(c.Key("folder-back")).Absolute().Left(x).Top(y-36).Padding(5, 9).Radius(8).Background(ui.RGBA(255, 255, 255, .08)).Gap(5).AlignItems(ui.Center).Label(v.t("native.folder.back")).Children(func() {
		ui.Icon(c, leftIcon).Size(12, 12)
		ui.Text(c, v.t("native.folder.back")).FontSize(12)
	}).OnClick(func() { v.selectCategory("all") })
}

var folderGlyph = svg(`<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v10H3Z"/>`)
