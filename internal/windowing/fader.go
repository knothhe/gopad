package windowing

import "time"

type FadingWindow interface {
	SetOpacity(float64)
	Opacity() float64
	Show()
	Hide()
	IsVisible() bool
}

// Fader animates only the temporary launchpad window. Call its methods on the
// UI thread; post sends timer callbacks back to that same thread.
type Fader struct {
	window     FadingWindow
	post       func(func())
	now        func() time.Time
	after      func(time.Duration, func())
	generation uint64
	desired    bool
	animating  bool
}

func NewFader(window FadingWindow, post func(func())) *Fader {
	return &Fader{window: window, post: post, now: time.Now, after: func(delay time.Duration, fn func()) { time.AfterFunc(delay, fn) }}
}

// DesiredVisible reports the requested state, including during a fade.
// Call it on the same UI thread as Show and Hide.
func (f *Fader) DesiredVisible() bool { return f.desired }

func (f *Fader) Show(reduceMotion bool) {
	if f.desired && f.window.IsVisible() {
		f.window.Show()
		return
	}
	f.generation++
	f.desired = true
	f.animating = false
	if !f.window.IsVisible() {
		f.window.SetOpacity(0)
	}
	f.window.Show()
	if reduceMotion {
		f.window.SetOpacity(1)
		return
	}
	f.animate(1, 160*time.Millisecond)
}

func (f *Fader) Hide(reduceMotion bool) {
	if !f.desired && f.animating {
		return
	}
	f.generation++
	f.desired = false
	f.animating = false
	if reduceMotion || !f.window.IsVisible() || f.window.Opacity() <= 0 {
		f.window.SetOpacity(0)
		f.window.Hide()
		return
	}
	f.animate(0, 120*time.Millisecond)
}

func (f *Fader) HideImmediately() {
	f.Stop()
	f.desired = false
	f.window.SetOpacity(0)
	f.window.Hide()
}

// Stop invalidates queued timer callbacks, including one already posted to the
// UI thread, so a stale fade-out can never hide a newly opened launchpad.
func (f *Fader) Stop() { f.generation++; f.animating = false }

func (f *Fader) animate(target float64, duration time.Duration) {
	start := f.window.Opacity()
	if start == target {
		if target == 0 {
			f.window.Hide()
		}
		return
	}
	generation := f.generation
	started := f.now()
	f.animating = true
	var tick func()
	tick = func() {
		if generation != f.generation {
			return
		}
		progress := float64(f.now().Sub(started)) / float64(duration)
		if progress >= 1 {
			f.animating = false
			f.window.SetOpacity(target)
			if target == 0 {
				f.window.Hide()
			}
			return
		}
		progress = max(0, progress)
		eased := progress * progress * (3 - 2*progress)
		f.window.SetOpacity(start + (target-start)*eased)
		f.after(16*time.Millisecond, func() { f.post(tick) })
	}
	f.after(16*time.Millisecond, func() { f.post(tick) })
}
