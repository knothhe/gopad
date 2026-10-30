package launchview

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/launcher"
)

func TestCategoryBarAlignsToVisibleIconPlateAtEverySize(t *testing.T) {
	for _, test := range []struct{ width, height, size int }{{700, 520, 48}, {1440, 900, 112}, {2560, 1440, 80}} {
		t.Run(fmt.Sprintf("%dpx-%dw", test.size, test.width), func(t *testing.T) {
			f := fixture(70)
			f.state.Preferences.IconSize = test.size
			v := testPad(f, Callbacks{})
			defer v.Close()
			tt := ui.NewTester(v.View, test.width, test.height)
			bar, found := tt.Find("应用分类")
			if !found {
				t.Fatal("missing multi-category strip")
			}
			tile, found := tt.Find("工具 000")
			if !found {
				t.Fatal("missing first application")
			}
			size := float32(test.size)
			visibleLeft := tile.X + (tile.W-size)/2 + iconOpticalInset(size)
			if math.Abs(float64(bar.X-visibleLeft)) > .1 {
				t.Fatalf("bar x=%g does not match first icon's visible edge %g (tile=%+v)", bar.X, visibleLeft, tile)
			}
			geometry := layout(float32(test.width), float32(test.height), size)
			_, _, maxWidth := categoryGeometry(geometry, size)
			visibleRight := tile.X + float32(geometry.Columns-1)*geometry.CellW + (geometry.CellW+size)/2 - iconOpticalInset(size)
			if math.Abs(float64(bar.X+maxWidth-visibleRight)) > .1 {
				t.Fatalf("maximum category width does not end at last visible icon: x=%g max=%g edge=%g", bar.X, maxWidth, visibleRight)
			}
			if bar.X+bar.W > visibleRight+.1 || bar.X+maxWidth > visibleRight+.1 {
				t.Fatalf("category strip extends past final visible icon edge: bar=%+v maximum=%g visibleRight=%g", bar, maxWidth, visibleRight)
			}
		})
	}
}

func TestSingleVisibleCategoryHasNoTabContainerAndKeepsGridStable(t *testing.T) {
	managed := 0
	v := testPad(fixture(8), Callbacks{OpenManager: func() { managed++ }})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	before, found := tt.Find("工具 000")
	if !found {
		t.Fatal("missing first app before changing category visibility")
	}
	v.mu.Lock()
	v.state.Preferences.Categories = launcher.DefaultCategories()
	for i := range v.state.Preferences.Categories {
		v.state.Preferences.Categories[i].Visible = i == 0
	}
	v.mu.Unlock()
	tt.Frame()
	if _, found := tt.Find("应用分类"); found {
		t.Fatal("single category still created a tab container")
	}
	if _, found := tt.Find("分类：全部"); found {
		t.Fatal("single category still created a tab label")
	}
	if tt.HasText("Tab 切换 · ⇧Tab 返回") {
		t.Fatal("single category still shows a category-switch hint")
	}
	after, found := tt.Find("工具 000")
	if !found || after.X != before.X || after.Y != before.Y {
		t.Fatalf("removing the strip moved the grid: before=%+v after=%+v", before, after)
	}
	tt.Key(0, ui.KeyTab)
	if v.focus != "app-000" {
		t.Fatalf("ordinary Tab did not reach the first app: focus=%s", v.focus)
	}
	if err := tt.Click("管理应用"); err != nil {
		t.Fatal(err)
	}
	if managed != 1 {
		t.Fatal("management gear is not reachable without category tabs")
	}
}
