// Package uikit provides gopad's reusable native Liquid Glass controls.
package uikit

import (
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

type Style uint8

const (
	Regular Style = iota
	Clear
)

func Surface(c *ui.Context, radius float32, style Style) ui.Element {
	material := glass.Glass{Style: glass.Regular, Tint: ui.RGBA(255, 255, 255, .04)}
	if style == Clear {
		material.Style = glass.Clear
		material.Tint = ui.Transparent
	}
	if c.Preferences().HighContrast {
		material.Style = glass.Regular
		material.Tint = ui.RGBA(0, 0, 0, .20)
	}
	surface := ui.Row(c).Radius(radius).Material(material)
	if c.Preferences().HighContrast {
		surface.Border(1, ui.RGBA(255, 255, 255, .34))
	}
	return surface
}
func Pill(c *ui.Context, width, height float32, style Style) ui.Element {
	return Surface(c, height/2, style).Size(width, height)
}
func BackdropBlur(c *ui.Context, radius float32) ui.Element {
	return ui.Box(c).Material(glass.Blur{Radius: radius}).PassThrough()
}
func IconButton(c *ui.Context, key, label string, svg *ui.SVG) ui.Element {
	material := glass.Glass{Style: glass.Clear, Interactive: true}
	if c.Preferences().HighContrast {
		material.Style = glass.Regular
		material.Tint = ui.RGBA(0, 0, 0, .20)
	}
	button := ui.ButtonBase(c.Key(key)).Size(38, 38).Center().Radius(19).Material(material).Label(label).Tooltip(label)
	if c.Preferences().HighContrast {
		button.Border(1, ui.RGBA(255, 255, 255, .34))
	}
	return button.Children(func() { ui.Icon(c, svg).Size(16, 16).TextColor(ui.RGBA(255, 255, 255, .92)) })
}

type Tab struct{ ID, Label, AccessibleLabel string }
type TabsLabels struct{ Group, Previous, Next string }
type TabLayout struct {
	Start, End int
	Width      float32
	Widths     []float32
	Arrows     bool
}

func tabWidth(label string) float32 {
	return max(68, min(160, 32+float32(utf8.RuneCountInString(label))*10))
}

// LayoutTabs keeps the active category visible and every control in bounds.
// Arrow buttons reveal/select neighboring categories when all labels do not fit.
func LayoutTabs(tabs []Tab, active string, maxWidth float32) TabLayout {
	maxWidth = max(120, maxWidth)
	if len(tabs) == 0 {
		return TabLayout{}
	}
	widths := make([]float32, len(tabs))
	total := float32(6 + max(0, len(tabs)-1)*3)
	index := 0
	for i, tab := range tabs {
		widths[i] = tabWidth(tab.Label)
		total += widths[i]
		if tab.ID == active {
			index = i
		}
	}
	if total <= maxWidth {
		return TabLayout{Start: 0, End: len(tabs), Width: total, Widths: widths}
	}
	budget := maxWidth - 68
	for i := range widths {
		widths[i] = min(widths[i], budget)
	}
	start, end := index, index+1
	used := widths[index]
	// Grow alternately so the active item remains near the center.
	for {
		changed := false
		if start > 0 && used+widths[start-1]+3 <= budget {
			start--
			used += widths[start] + 3
			changed = true
		}
		if end < len(tabs) && used+widths[end]+3 <= budget {
			used += widths[end] + 3
			end++
			changed = true
		}
		if !changed {
			break
		}
	}
	return TabLayout{Start: start, End: end, Width: used + 68, Widths: widths[start:end], Arrows: true}
}

// Tabs is a compact glass category strip. It does not handle Tab keys itself;
// the launcher's search/grid scope owns category keyboard navigation.
func Tabs(c *ui.Context, tabs []Tab, active string, maxWidth float32, onSelect func(string), labels TabsLabels, decorate ...func(ui.Element, string)) ui.Element {
	layout := LayoutTabs(tabs, active, maxWidth)
	activeIndex := 0
	for i, tab := range tabs {
		if tab.ID == active {
			activeIndex = i
			break
		}
	}
	return Surface(c, 11, Clear).Size(layout.Width, 35).Padding(3).Gap(3).AlignItems(ui.Center).Label(labels.Group).OnClick(func() {}).Children(func() {
		if layout.Arrows {
			ui.ButtonBase(c.Key("previous-category")).Size(28, 29).Center().Radius(8).Label(labels.Previous).Disabled(activeIndex == 0).Children(func() { ui.Icon(c, left).Size(12, 12) }).OnClick(func() {
				if activeIndex > 0 {
					onSelect(tabs[activeIndex-1].ID)
				}
			})
		}
		for i := layout.Start; i < layout.End; i++ {
			tab := tabs[i]
			accessible := tab.AccessibleLabel
			if accessible == "" {
				accessible = tab.Label
			}
			button := ui.ButtonBase(c.Key(tab.ID)).Size(layout.Widths[i-layout.Start], 29).Radius(8).Border(1, ui.Transparent).Label(accessible).Tooltip(tab.Label)
			color := ui.Transparent
			if tab.ID == active {
				color = ui.RGBA(255, 255, 255, .115)
				button.Border(1, ui.RGBA(255, 255, 255, .15))
			} else if button.Hovered() {
				color = ui.RGBA(255, 255, 255, .045)
			}
			button.Background(button.AnimateColor("active", color, ui.Animation{Duration: 130 * time.Millisecond})).Children(func() {
				alpha := float32(.57)
				if tab.ID == active {
					alpha = 1
				} else if c.Preferences().HighContrast {
					alpha = .86
				}
				ui.Text(c, tab.Label).Width(layout.Widths[i-layout.Start] - 18).SingleLine().Ellipsis("…").TextAlign(ui.Center).FontSize(12).TextColor(ui.RGBA(255, 255, 255, alpha))
			}).OnClick(func() { onSelect(tab.ID) })
			for _, hook := range decorate {
				hook(button, tab.ID)
			}
		}
		if layout.Arrows {
			ui.ButtonBase(c.Key("next-category")).Size(28, 29).Center().Radius(8).Label(labels.Next).Disabled(activeIndex == len(tabs)-1).Children(func() { ui.Icon(c, right).Size(12, 12) }).OnClick(func() {
				if activeIndex+1 < len(tabs) {
					onSelect(tabs[activeIndex+1].ID)
				}
			})
		}
	})
}
func glyph(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

var left = glyph(`<path d="m14 6-6 6 6 6"/>`)
var right = glyph(`<path d="m10 6 6 6-6 6"/>`)
