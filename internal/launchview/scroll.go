package launchview

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

const swipeIdle = 100 * time.Millisecond

type scrollPhase uint8

const (
	scrollUnphased scrollPhase = iota
	scrollBegan
	scrollChanged
	scrollEnded
	scrollCancelled
)

type pageScroll struct {
	last, motion     time.Time
	dx, dy, velocity float32
	axis             int
	active, phased   bool
}

func (v *Launchpad) handlePageScroll(ev ui.InputEvent, now time.Time) bool {
	return v.handlePageScrollPhase(ev, now, nativeScrollPhase())
}

func (v *Launchpad) handlePageScrollPhase(ev ui.InputEvent, now time.Time, phase scrollPhase) bool {
	if ev.Kind != ui.InputScroll || v.pageCount <= 1 {
		return false
	}
	if !ev.Precise {
		if ev.Momentum || now.Sub(v.lastWheel) < 350*time.Millisecond {
			return false
		}
		delta := ev.DY
		if abs(ev.DX) > abs(delta) {
			delta = ev.DX
		}
		if abs(delta) < 8 {
			return false
		}
		v.finishPageScroll(now, true)
		v.lastWheel = now
		if delta > 0 {
			v.changePage(v.page + 1)
		} else {
			v.changePage(v.page - 1)
		}
		return true
	}
	s := &v.scroll
	if ev.Momentum {
		v.finishPageScroll(now, false)
		return true
	}
	if phase == scrollBegan || (!s.phased && now.Sub(s.last) > swipeIdle) {
		v.finishPageScroll(now, false)
		*s = pageScroll{}
	}
	if phase != scrollUnphased {
		s.phased = true
	}
	elapsed := now.Sub(s.last)
	s.last = now
	s.dx += ev.DX
	s.dy += ev.DY
	if s.axis == 0 && max(abs(s.dx), abs(s.dy)) >= 4 {
		s.axis = -1
		if abs(s.dx) > abs(s.dy)*1.2 {
			s.axis = 1
		}
	}
	if s.axis == 1 && v.slides.ready {
		delta := ev.DX
		if !s.active {
			delta = s.dx
			s.active = true
		}
		if delta != 0 {
			dt := max(float32(.008), min(float32(.05), float32(elapsed.Seconds())))
			speed := delta / dt
			s.velocity = .65*speed + .35*s.velocity
			s.motion = now
			v.slides.dragBy(delta, now, v.pageRequest)
			if v.page != v.slides.last.key.page {
				v.page = v.slides.last.key.page
				v.selected = -1
				v.focus = "search"
			}
		}
	}
	if phase == scrollEnded || phase == scrollCancelled {
		v.finishPageScroll(now, phase == scrollCancelled)
		*s = pageScroll{}
	}
	// Consume small horizontal movements too, so every delta requests a frame.
	return s.axis != -1
}

func abs(value float32) float32 { return float32(math.Abs(float64(value))) }

func (v *Launchpad) pageRequest(page int) sceneRequest {
	request := v.slides.last
	state, _, _ := v.snapshot()
	if state.Preferences.LoopPages {
		request.key.page = (page%v.pageCount + v.pageCount) % v.pageCount
	} else {
		request.key.page = max(0, min(page, v.pageCount-1))
	}
	start := request.key.page * v.pageSize
	request.apps = v.filtered[start:min(len(v.filtered), start+v.pageSize)]
	return request
}

func (v *Launchpad) finishPageScroll(now time.Time, cancelled bool) {
	s, d := &v.scroll, &v.slides
	if !s.active || !d.dragging {
		s.active = false
		return
	}
	distance := d.from - d.to
	velocity := s.velocity / d.last.width
	velocityTime := now
	if !s.phased {
		velocityTime = s.last
	}
	if velocityTime.Sub(s.motion) > 80*time.Millisecond {
		velocity = 0
	}
	projected := distance + velocity*.15
	direction := 0
	if !cancelled && abs(distance) >= .015 && (abs(distance) >= .22 || abs(projected) >= .22 && projected*distance > 0) {
		direction = 1
		if distance < 0 {
			direction = -1
		}
	}
	if direction != 0 {
		request := v.pageRequest(v.page + direction)
		if request.key.page != v.page {
			v.changePage(v.page + direction)
			d.sync(request, direction, now, false, false)
		}
	}
	d.dragging = false
	d.started = now
	s.active = false
}
