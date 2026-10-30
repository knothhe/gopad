package launchview

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/launcher"
)

func slideRequest(category string, page int, apps []launcher.App) sceneRequest {
	return sceneRequest{key: sceneKey{category, page}, apps: apps, width: 900, height: 650, size: 80, capacity: 10, pageCount: 3, loaded: true}
}

func closeOffset(t *testing.T, got, want float32) {
	t.Helper()
	if math.Abs(float64(got-want)) > .0001 {
		t.Fatalf("offset %g, want %g", got, want)
	}
}

func TestPageAndCategorySlidesShareEveryFrameAndDuration(t *testing.T) {
	now := time.Unix(100, 0)
	apps := fixture(4).state.Apps
	var page, category slideDeck
	initial := slideRequest("a", 0, apps[:2])
	page.sync(initial, 0, now, false, false)
	category.sync(initial, 0, now, false, false)
	page.sync(slideRequest("a", 1, apps[2:]), 0, now, false, false)
	category.sync(slideRequest("b", 0, apps[2:]), 1, now, false, false)
	for _, elapsed := range []time.Duration{0, 55 * time.Millisecond, 110 * time.Millisecond, 219 * time.Millisecond, 220 * time.Millisecond} {
		p, pw := page.frames(now.Add(elapsed))
		c, cw := category.frames(now.Add(elapsed))
		if len(p) != len(c) || pw != cw {
			t.Fatalf("at %s page/category disagree: layers %d/%d, remaining %s/%s", elapsed, len(p), len(c), pw, cw)
		}
		for i := range p {
			closeOffset(t, p[i].offset, c[i].offset)
			if p[i].target != c[i].target {
				t.Fatal("page/category disagree about the interactive layer")
			}
		}
		if elapsed < contentSlide.Duration {
			progress := ui.EaseOut(float32(elapsed) / float32(contentSlide.Duration))
			if len(c) != 2 || c[0].target || !c[1].target {
				t.Fatalf("expected outgoing and incoming at %s: %+v", elapsed, c)
			}
			closeOffset(t, c[0].offset, -progress)
			closeOffset(t, c[1].offset, 1-progress)
		} else if len(c) != 1 || !c[0].target || c[0].offset != 0 || cw != 0 {
			t.Fatal("slide did not settle at exactly 220ms")
		}
	}
}

func TestRapidSlidesPreserveVisiblePixelsAndBoundHistory(t *testing.T) {
	now := time.Unix(100, 0)
	apps := fixture(4).state.Apps
	requests := []sceneRequest{slideRequest("a", 0, apps[:1]), slideRequest("b", 0, apps[1:2]), slideRequest("c", 0, apps[2:3]), slideRequest("d", 0, apps[3:])}
	var deck slideDeck
	deck.sync(requests[0], 0, now, false, false)
	firstID := deck.targetID
	deck.sync(requests[1], 1, now, false, false)
	now = now.Add(35 * time.Millisecond)
	before, _ := deck.frames(now)
	deck.sync(requests[2], 1, now, false, false)
	after, _ := deck.frames(now)
	for _, old := range before {
		for _, current := range after {
			if old.scene.id == current.scene.id {
				closeOffset(t, current.offset, old.offset)
			}
		}
	}
	// Reverse while the original scene is still visible: reuse its pixels
	// and its identity, rather than mounting another copy on the other side.
	now = now.Add(time.Millisecond)
	camera := deck.position(now)
	deck.sync(requests[0], -1, now, false, false)
	if deck.targetID != firstID {
		t.Fatal("reversal recreated a still-visible scene")
	}
	closeOffset(t, deck.position(now), camera)
	for i := 0; i < 1000; i++ {
		now = now.Add(3 * time.Millisecond)
		direction := 1
		if i%5 == 0 {
			direction = -1
		}
		deck.sync(requests[i%len(requests)], direction, now, false, false)
		frames, _ := deck.frames(now)
		if len(deck.layers) > 3 || len(frames) > 3 {
			t.Fatalf("rapid switching accumulated historical scenes: %d", len(deck.layers))
		}
	}
	frames, wake := deck.frames(now.Add(contentSlide.Duration))
	if len(frames) != 1 || !frames[0].target || frames[0].scene.request.key != deck.last.key || wake != 0 {
		t.Fatal("rapid switching settled on a stale selection")
	}
}

func transitionFixture() *fixtureCatalog {
	f := fixture(12)
	f.state.Preferences.Categories = launcher.DefaultCategories()
	for i := range f.state.Preferences.Categories {
		f.state.Preferences.Categories[i].Visible = false
	}
	for i, id := range []string{"a", "b", "c"} {
		category := launcher.Category{ID: id, Name: id, Visible: true}
		for _, app := range f.state.Apps[i*4 : (i+1)*4] {
			category.AppIDs = append(category.AppIDs, app.ID)
		}
		f.state.Preferences.Categories = append(f.state.Preferences.Categories, category)
	}
	f.state.Preferences.LastCategory = "a"
	return f
}

func TestCategorySlideClearsQueryAndRespectsWrapDirection(t *testing.T) {
	v := testPad(transitionFixture(), Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	tt.Type("工具 002")
	tt.Key(0, ui.KeyTab)
	frames, _ := v.slides.frames(tt.Now())
	if v.query != "" || v.category != "b" || len(frames) != 2 || frames[0].scene.request.apps[0].ID != "app-002" || frames[1].scene.request.apps[0].ID != "app-004" {
		t.Fatalf("query clearing swallowed category motion: category=%s query=%q frames=%+v", v.category, v.query, frames)
	}
	if v.slides.to <= v.slides.from {
		t.Fatal("forward Tab slid backward")
	}
	tt.Advance(contentSlide.Duration)
	tt.Key(0, ui.KeyTab)
	tt.Advance(contentSlide.Duration)
	tt.Key(0, ui.KeyTab) // c -> a is forward despite wrapping.
	if v.category != "a" || v.slides.to <= v.slides.from {
		t.Fatal("forward wrap did not continue in the Tab direction")
	}
	tt.Advance(contentSlide.Duration)
	tt.Key(ui.Shift, ui.KeyTab)
	if v.category != "c" || v.slides.to >= v.slides.from {
		t.Fatal("reverse wrap did not continue in the ShiftTab direction")
	}
}

func TestOutgoingSceneCannotLaunchAndKeepsIconOpacity(t *testing.T) {
	f := transitionFixture()
	v := testPad(f, Callbacks{})
	defer v.Close()
	icon := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(icon, icon.Bounds(), image.NewUniform(color.RGBA{R: 240, G: 30, B: 20, A: 255}), image.Point{}, draw.Src)
	for _, app := range f.state.Apps {
		v.icons[app.ID] = ui.NewBitmap(icon)
	}
	tt := ui.NewTester(v.View, 900, 650)
	tt.Type("工具 002")
	tt.Key(0, ui.KeyTab)
	tt.Advance(10 * time.Millisecond)
	label, ok := tt.Find("工具 002")
	if !ok {
		t.Fatal("outgoing scene disappeared before the slide completed")
	}
	x := label.X + label.W/2
	y := label.Y - 9 - 40
	pixel := tt.Image().RGBAAt(int(x), int(y))
	if pixel.R < 235 || pixel.G > 35 {
		t.Fatalf("outgoing icon was dimmed: %+v", pixel)
	}
	tt.ClickAt(x, y)
	select {
	case id := <-f.launched:
		t.Fatalf("outgoing app was interactive: %s", id)
	default:
	}
	tt.Key(0, ui.KeyEnter)
	select {
	case id := <-f.launched:
		if id != "app-004" {
			t.Fatalf("Enter opened outgoing selection %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("incoming selection could not launch during its slide")
	}
}

func TestReducedMotionHasOneStableInteractiveScene(t *testing.T) {
	f := transitionFixture()
	v := testPad(f, Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	tt.Key(0, ui.KeyTab)
	frames, wake := v.slides.frames(tt.Now())
	id := v.slides.targetID
	if len(frames) != 1 || frames[0].offset != 0 || !frames[0].target || wake != 0 {
		t.Fatal("reduced motion left an outgoing or moving scene")
	}
	tt.Frame()
	if v.slides.targetID != id {
		t.Fatal("reduced motion recreated stable scene keys every frame")
	}
	if err := tt.Click("工具 004"); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-f.launched:
		if id != "app-004" {
			t.Fatalf("clicked %s", id)
		}
	case <-time.After(time.Second):
		t.Fatal("reduced-motion scene lost its click during rebuild")
	}
}

func TestRenderCategorySlidePreview(t *testing.T) {
	if os.Getenv("GOPAD_RENDER_TRANSITION") != "1" {
		t.Skip("set GOPAD_RENDER_TRANSITION=1 for native slide keyframes")
	}
	service, err := launcher.New(filepath.Join(t.TempDir(), "preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	state, err := service.State()
	if err != nil {
		t.Fatal(err)
	}
	state.Preferences.Categories = launcher.DefaultCategories()
	state.Preferences.Categories[0].Name = "全部"
	state.Preferences.Favorites = nil
	apps := appsFor(state, "")
	for _, app := range apps[len(apps)/2:] {
		state.Preferences.Favorites = append(state.Preferences.Favorites, app.ID)
	}
	v := New(service, Callbacks{})
	defer v.Close()
	v.state, v.loaded = state, true
	if data, err := launcher.Wallpaper(); err == nil {
		if err := v.SetWallpaper(data); err != nil {
			t.Fatal(err)
		}
	}
	for _, app := range apps {
		if data, err := service.Icon(app.ID); err == nil {
			if bitmap, err := ui.DecodeBitmap(data); err == nil {
				v.icons[app.ID] = bitmap
			}
		}
	}
	tt := ui.NewTester(v.View, 1440, 900)
	write := func(name string) {
		t.Helper()
		path := filepath.Join("/tmp", name)
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, tt.Image()); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		t.Log(path)
	}
	write("gopad-category-slide-start.png")
	tt.Key(0, ui.KeyTab)
	tt.Advance(55 * time.Millisecond)
	write("gopad-category-slide-middle.png")
	tt.Advance(165 * time.Millisecond)
	write("gopad-category-slide-end.png")
}
