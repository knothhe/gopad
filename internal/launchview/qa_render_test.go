package launchview

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/launcher"
)

// TestRenderSyntheticLauncher is an opt-in visual QA surface. It does not scan
// the computer, read wallpaper, or inspect installed application metadata.
func TestRenderSyntheticLauncher(t *testing.T) {
	dir := os.Getenv("GOPAD_RENDER_QA_DIR")
	if dir == "" {
		t.Skip("set GOPAD_RENDER_QA_DIR to render synthetic launcher QA images")
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("GOPAD_RENDER_QA_DIR must be an absolute directory")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, size := range [][2]int{{700, 520}, {1440, 900}} {
		for _, mode := range []string{"browse", "search", "feedback", "folders", "folder-open"} {
			t.Run(fmt.Sprintf("%dx%d-%s", size[0], size[1], mode), func(t *testing.T) {
				f := fixture(84)
				f.state.Preferences.SearchScope = "all"
				f.state.Preferences.SearchRanking = "relevance"
				f.state.Preferences.Categories = launcher.DefaultCategories()
				for i, name := range []string{"开发工具", "设计创作", "工作与沟通", "非常长的自定义分区名称用于检查溢出"} {
					members := []string{}
					for j := i * 6; j < (i+1)*6; j++ {
						members = append(members, f.state.Apps[j].ID)
					}
					f.state.Preferences.Categories = append(f.state.Preferences.Categories, launcher.Category{ID: fmt.Sprint("qa-", i), Name: name, AppIDs: members, Visible: true})
				}
				if mode == "folders" || mode == "folder-open" {
					f.state.Preferences.ShowCategoryFolders = true
				}
				names := []string{"极速笔记", "设计工具箱", "终端工具", "个人知识库", "浏览器", "视频剪辑", "Audio Studio", "Project Workspace", "开发工具", "计算器", "日历", "系统设置"}
				for i := range f.state.Apps {
					f.state.Apps[i].Name = names[i%len(names)]
					if i >= len(names) {
						f.state.Apps[i].Name += fmt.Sprint(" ", i/len(names)+1)
					}
				}
				v := testPad(f, Callbacks{})
				defer v.Close()
				for i, app := range f.state.Apps {
					v.icons[app.ID] = syntheticQAIcon(t, i)
				}
				if mode == "folder-open" {
					v.category, v.categoryRestored = "qa-0", true
				}
				if mode == "search" {
					v.query = "工具"
					v.queryChanged()
				}
				if mode == "feedback" {
					v.actions.undoID, v.actions.undoName = "app-005", "视频剪辑"
					v.errKey, v.errDetail = "native.error.action", "无法保存更改，请重试"
				}
				tt := ui.NewTester(v.View, size[0], size[1])
				output := filepath.Join(dir, fmt.Sprintf("launcher-%dx%d-%s.png", size[0], size[1], mode))
				file, err := os.Create(output)
				if err != nil {
					t.Fatal(err)
				}
				if err := png.Encode(file, tt.Image()); err != nil {
					file.Close()
					t.Fatal(err)
				}
				if err := file.Close(); err != nil {
					t.Fatal(err)
				}
				t.Log(output)
			})
		}
	}
}

func syntheticQAIcon(t *testing.T, index int) *ui.Bitmap {
	t.Helper()
	palette := []color.RGBA{{75, 140, 245, 255}, {228, 115, 63, 255}, {126, 88, 205, 255}, {55, 172, 128, 255}, {211, 91, 133, 255}, {70, 166, 193, 255}}
	base := palette[index%len(palette)]
	canvas := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := 25; y < 231; y++ {
		for x := 25; x < 231; x++ {
			dx, dy := max(0, max(64-x, x-191)), max(0, max(64-y, y-191))
			if dx*dx+dy*dy > 39*39 {
				continue
			}
			value := base
			value.R = uint8(min(255, int(value.R)+(230-y)/8))
			value.G = uint8(min(255, int(value.G)+(230-y)/8))
			value.B = uint8(min(255, int(value.B)+(230-y)/8))
			if x > 79 && x < 177 && y > 79 && y < 177 && (x < 91 || x > 164 || y < 91 || y > 164 || index%2 == 0 && x > 120 && x < 136) {
				value = color.RGBA{255, 255, 255, 235}
			}
			canvas.SetRGBA(x, y, value)
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		t.Fatal(err)
	}
	bitmap, err := ui.DecodeBitmap(encoded.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return bitmap
}
