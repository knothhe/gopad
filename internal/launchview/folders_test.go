package launchview

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/launcher"
)

func folderFixture() launcher.State {
	state := fixture(5).state
	state.Preferences.ShowCategoryFolders = true
	state.Preferences.Categories = append(launcher.DefaultCategories(),
		launcher.Category{ID: "work", Name: "Work", AppIDs: []string{"app-000", "app-001", "app-002"}, Visible: true},
		launcher.Category{ID: "tools", Name: "Tools", AppIDs: []string{"app-001", "app-003"}, Visible: true},
		launcher.Category{ID: "private", Name: "Private", AppIDs: []string{"app-004"}, Visible: false},
	)
	state.Preferences.Hidden = []string{"app-002"}
	return state
}

func TestFolderPresentationPreservesMembershipAndHiddenApps(t *testing.T) {
	state := folderFixture()
	all := launcher.Category{ID: "all"}
	apps := folderApps(state, appsForCategory(state, "", all), "", all)
	var ids []string
	for _, app := range apps {
		ids = append(ids, app.ID)
	}
	if want := []string{"folder:work", "folder:tools", "app-004"}; !reflect.DeepEqual(ids, want) {
		t.Fatalf("folder tiles = %v, want %v", ids, want)
	}
	f := &fixtureCatalog{state: state}
	v := New(f, Callbacks{})
	defer v.Close()
	v.state, v.loaded = state, true
	for _, check := range []struct {
		folder launcher.App
		want   []string
	}{
		{apps[0], []string{"app-000", "app-001"}},
		{apps[1], []string{"app-001", "app-003"}},
	} {
		var members []string
		for _, app := range v.folderMembers(check.folder) {
			members = append(members, app.ID)
		}
		if !reflect.DeepEqual(members, check.want) {
			t.Fatalf("members of %s = %v, want %v", check.folder.ID, members, check.want)
		}
	}
}

func TestFoldersNeverChangeSearchOrCategoryContents(t *testing.T) {
	state := folderFixture()
	all := launcher.Category{ID: "all"}
	searched := appsForCategory(state, "工具", all)
	if got := folderApps(state, searched, "工具", all); !reflect.DeepEqual(got, searched) || len(got) != 4 {
		t.Fatalf("search lost folder members or included hidden app: %v", got)
	}
	category := state.Preferences.Categories[4]
	members := appsForCategory(state, "", category)
	if got := folderApps(state, members, "", category); !reflect.DeepEqual(got, members) {
		t.Fatal("custom category was transformed into nested folders")
	}
	state.Preferences.ShowCategoryFolders = false
	if got := folderApps(state, searched, "", all); !reflect.DeepEqual(got, searched) {
		t.Fatal("disabled folder option changed browse results")
	}
}

func TestFolderOpenAndKeyboardLaunchRouteToCategory(t *testing.T) {
	f := fixture(5)
	f.state = folderFixture()
	v := testPad(f, Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	if v.filtered[0].ID != "folder:work" {
		t.Fatalf("folder is not the first browse result: %v", v.filtered)
	}
	tt.Key(0, ui.KeyDown)
	tt.Key(0, ui.KeyEnter)
	if v.category != "work" {
		t.Fatalf("Enter did not open selected folder: %s", v.category)
	}
	select {
	case id := <-f.launched:
		t.Fatalf("folder was sent to application launch API: %s", id)
	default:
	}
	if err := tt.Click(v.t("native.folder.back")); err != nil {
		t.Fatal(err)
	}
	if v.category != "all" {
		t.Fatal("folder back button did not restore all apps")
	}
}

func TestFolderDropAddsMembershipAndContextMenuOpensFolder(t *testing.T) {
	v, f := actionPad(t)
	folder := launcher.App{ID: "folder:work", Name: "Work", Kind: "folder"}
	v.state.Preferences.Categories = append(launcher.DefaultCategories(), launcher.Category{ID: "work", Name: "Work", Visible: true})
	v.categories = launcher.VisibleCategories(v.state.Preferences)
	managed := 0
	v.callbacks.OpenManager = func() { managed++ }
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Row(c).Gap(20).Children(func() {
			source := ui.ButtonBase(c.Key("source")).Size(100, 100).Label("Source app")
			v.bindAppActions(source, f.state.Apps[0])
			tile := ui.ButtonBase(c.Key("folder")).Size(100, 100).Label("Work folder")
			if !v.bindFolderActions(tile, folder) {
				t.Fatal("folder binder did not recognize folder")
			}
		})
	}, 300, 200)
	from, _ := tt.Find("Source app")
	to, _ := tt.Find("Work folder")
	tt.Press(from.X+10, from.Y+10)
	tt.Move(to.X+20, to.Y+20)
	tt.Release(to.X+20, to.Y+20)
	if call := awaitAction(t, v, f); call != (appActionCall{"category", "app-000", "work"}) {
		t.Fatalf("folder drop = %+v", call)
	}
	if err := tt.RightClick("Work folder"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(v.t("native.manage")); err != nil {
		t.Fatal(err)
	}
	if managed != 1 {
		t.Fatal("folder management menu did not open manager")
	}
	v.categories = append(v.categories, launcher.Category{ID: "work", Name: "Work", Visible: true})
	if err := tt.RightClick("Work folder"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem(v.t("native.menu.open")); err != nil {
		t.Fatal(err)
	}
	if v.category != "work" {
		t.Fatal("folder context menu did not open category")
	}
}
