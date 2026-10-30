package uikit

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestLongTabsKeepActiveItemWithinWidth(t *testing.T) {
	tabs := make([]Tab, 50)
	for i := range tabs {
		tabs[i] = Tab{ID: fmt.Sprint(i), Label: fmt.Sprintf("非常长的自定义分类名称第%d个项目和工具", i)}
	}
	for _, width := range []float32{120, 320, 636, 720} {
		for i, tab := range tabs {
			layout := LayoutTabs(tabs, tab.ID, width)
			if layout.Width > width || layout.Start > i || layout.End <= i {
				t.Fatalf("active %d width %g layout %+v", i, width, layout)
			}
			sum := float32(6 + max(0, len(layout.Widths)-1)*3)
			if layout.Arrows {
				sum += 62
			}
			for _, itemWidth := range layout.Widths {
				sum += itemWidth
			}
			if sum > layout.Width+.01 {
				t.Fatalf("children overflow tab strip: sum=%g layout=%+v", sum, layout)
			}
		}
	}
	active := tabs[0].ID
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(ui.DarkTheme())
		ui.Box(c).Fill().Background(ui.Hex("#354260"))
		Tabs(c, tabs, active, 320, func(id string) { active = id }, TabsLabels{Group: "分类", Previous: "上一分类", Next: "下一分类"}).Absolute().Left(20).Top(20)
	}, 360, 100)
	for i := 1; i < 5; i++ {
		if err := tt.Click("下一分类"); err != nil {
			t.Fatal(err)
		}
		if active != tabs[i].ID {
			t.Fatalf("next category %s, want %s", active, tabs[i].ID)
		}
	}
	if r, ok := tt.Find("下一分类"); !ok || r.X+r.W > 340 {
		t.Fatalf("native tab arrow overflows: %+v found=%v", r, ok)
	}
}
