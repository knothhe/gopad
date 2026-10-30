package windowing

import (
	"testing"
	"time"
)

type fadingFixture struct {
	visible   bool
	opacity   float64
	hideCalls int
	shownAt   []float64
}

func (w *fadingFixture) SetOpacity(value float64) { w.opacity = value }
func (w *fadingFixture) Opacity() float64         { return w.opacity }
func (w *fadingFixture) Show()                    { w.visible = true; w.shownAt = append(w.shownAt, w.opacity) }
func (w *fadingFixture) Hide()                    { w.visible = false; w.hideCalls++ }
func (w *fadingFixture) IsVisible() bool          { return w.visible }
func fakeFader() (*Fader, *fadingFixture, *time.Time, *[]func()) {
	window := &fadingFixture{opacity: 1}
	clock := time.Unix(0, 0)
	callbacks := []func(){}
	fader := NewFader(window, func(fn func()) { fn() })
	fader.now = func() time.Time { return clock }
	fader.after = func(_ time.Duration, fn func()) { callbacks = append(callbacks, fn) }
	return fader, window, &clock, &callbacks
}
func finishFade(clock *time.Time, callbacks *[]func()) {
	*clock = clock.Add(time.Second)
	pending := *callbacks
	*callbacks = nil
	for _, fn := range pending {
		fn()
	}
}

func TestFadeShowsWithoutFlashAndHidesWithoutClosing(t *testing.T) {
	fader, window, clock, callbacks := fakeFader()
	fader.Show(false)
	if !window.visible || window.shownAt[0] != 0 {
		t.Fatalf("first frame flashed at opacity %v", window.shownAt)
	}
	finishFade(clock, callbacks)
	if window.opacity != 1 || !window.visible {
		t.Fatal("fade-in did not finish visible")
	}
	fader.Hide(false)
	if !window.visible {
		t.Fatal("window hid before fade-out finished")
	}
	finishFade(clock, callbacks)
	if window.visible || window.opacity != 0 || window.hideCalls != 1 {
		t.Fatalf("fade-out left state %+v", window)
	}
}

func TestStaleFadeOutCannotHideReopenedLauncher(t *testing.T) {
	fader, window, clock, callbacks := fakeFader()
	fader.Show(true)
	fader.Hide(false)
	oldCallbacks := append([]func(){}, (*callbacks)...)
	*callbacks = nil
	fader.Show(false)
	*clock = clock.Add(time.Second)
	for _, fn := range oldCallbacks {
		fn()
	}
	if !window.visible || window.hideCalls != 0 || window.opacity != 1 {
		t.Fatalf("old fade-out hid reopened launcher: %+v", window)
	}
}

func TestReducedMotionSkipsAnimationAndImmediateHideCancelsFade(t *testing.T) {
	fader, window, clock, callbacks := fakeFader()
	fader.Show(true)
	fader.Hide(true)
	if len(*callbacks) != 0 || window.visible {
		t.Fatal("reduced-motion action scheduled animation")
	}
	fader.Show(false)
	fader.HideImmediately()
	finishFade(clock, callbacks)
	if window.visible || window.opacity != 0 {
		t.Fatal("old fade-in restored immediately hidden launcher")
	}
}

func TestRapidToggleReversesFadeOutWhileWindowRemainsVisible(t *testing.T) {
	fader, window, clock, callbacks := fakeFader()
	focused := true
	toggle := func() {
		if fader.DesiredVisible() && focused {
			fader.Hide(false)
		} else {
			fader.Show(false)
		}
	}
	fader.Show(true)
	toggle()
	if fader.DesiredVisible() || !window.visible {
		t.Fatal("fade-out should be logically hidden while still drawing")
	}
	toggle()
	if !fader.DesiredVisible() {
		t.Fatal("a second toggle during fade-out did not request showing")
	}
	finishFade(clock, callbacks)
	if !window.visible || window.opacity != 1 || window.hideCalls != 0 {
		t.Fatalf("reversed toggle ended hidden: %+v", window)
	}
	// A visible launchpad without focus is brought forward by toggling.
	focused = false
	toggle()
	if !fader.DesiredVisible() || !window.visible {
		t.Fatal("toggle hid an unfocused launchpad")
	}
}
