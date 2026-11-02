package launchview

import (
	"math"
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/knothhe/gopad/internal/launcher"
)

// Categories and application pages share one camera and one motion curve.
// Scene snapshots remain immutable while the camera moves between them.
var contentSlide = ui.Animation{Duration: 220 * time.Millisecond, Ease: ui.EaseOut}

type sceneKey struct {
	category string
	page     int
}
type sceneRequest struct {
	key                 sceneKey
	apps                []launcher.App
	emptyKey            string
	query               string
	width, height, size float32
	capacity, pageCount int
	revision            uint64
	loaded              bool
}
type slideScene struct {
	id      uint64
	slot    float32
	request sceneRequest
}
type sceneFrame struct {
	scene  slideScene
	offset float32
	target bool
}
type slideDeck struct {
	ready            bool
	dragging         bool
	last             sceneRequest
	layers           []slideScene
	targetID, nextID uint64
	from, to         float32
	started          time.Time
}

func (d *slideDeck) position(now time.Time) float32 {
	if d.dragging {
		return d.from
	}
	if d.from == d.to {
		return d.to
	}
	progress := float32(now.Sub(d.started)) / float32(contentSlide.Duration)
	if progress >= 1 {
		return d.to
	}
	if progress <= 0 {
		return d.from
	}
	return d.from + (d.to-d.from)*contentSlide.Ease(progress)
}
func sameScene(a, b sceneRequest) bool {
	return a.key == b.key && a.emptyKey == b.emptyKey && slices.Equal(a.apps, b.apps)
}
func (d *slideDeck) reset(request sceneRequest, now time.Time) {
	d.nextID++
	request.apps = append([]launcher.App(nil), request.apps...)
	d.layers = []slideScene{{id: d.nextID, request: request}}
	d.targetID = d.nextID
	d.from = 0
	d.to = 0
	d.started = now
	d.ready = true
	d.dragging = false
	d.last = request
}
func (d *slideDeck) sync(request sceneRequest, direction int, now time.Time, reduceMotion, forceReset bool) {
	categoryChanged := d.ready && d.last.key.category != request.key.category
	contextChanged := !d.ready || d.last.width != request.width || d.last.height != request.height || d.last.size != request.size || d.last.capacity != request.capacity || d.last.revision != request.revision || d.last.loaded != request.loaded
	// Clearing a query is part of changing category and must not swallow its
	// slide. A standalone search or catalog/layout change replaces it at once.
	if !categoryChanged && (d.last.query != request.query || d.last.pageCount != request.pageCount) {
		contextChanged = true
	}
	if d.ready && d.last.key == request.key && !sameScene(d.last, request) {
		contextChanged = true
	}
	if forceReset || contextChanged || (reduceMotion && (request.key != d.last.key || d.from != d.to && !d.dragging)) {
		d.reset(request, now)
		return
	}
	if request.key == d.last.key {
		d.last = request
		return
	}
	camera := d.position(now)
	d.dragging = false
	if direction == 0 {
		direction = 1
		if request.key.category == d.last.key.category && request.key.page < d.last.key.page {
			direction = -1
		}
	}
	step := float32(1)
	if direction < 0 {
		step = -1
	}
	// Retain only pixels currently on screen. An intermediate selection
	// that never entered the viewport is obsolete, not another page to replay.
	visible := d.layers[:0]
	for _, layer := range d.layers {
		offset := layer.slot - camera
		if offset > -1 && offset < 1 {
			visible = append(visible, layer)
		}
	}
	d.layers = visible
	var target *slideScene
	best := float32(math.MaxFloat32)
	for i := range d.layers {
		layer := &d.layers[i]
		distance := float32(math.Abs(float64(layer.slot - camera)))
		if distance < best && sameScene(layer.request, request) {
			target = layer
			best = distance
		}
	}
	if target == nil {
		slot := float32(math.Ceil(float64(camera)))
		if step < 0 {
			slot = float32(math.Floor(float64(camera)))
		}
		if slot == camera {
			slot += step
		}
		// Never replace a visible scene at the incoming coordinate.
		for {
			occupied := false
			for _, layer := range d.layers {
				if layer.slot == slot {
					occupied = true
					break
				}
			}
			if !occupied {
				break
			}
			slot += step
		}
		d.nextID++
		request.apps = append([]launcher.App(nil), request.apps...)
		d.layers = append(d.layers, slideScene{id: d.nextID, slot: slot, request: request})
		target = &d.layers[len(d.layers)-1]
	}
	d.targetID = target.id
	d.from = camera
	d.to = target.slot
	d.started = now
	d.last = request
}

// dragBy moves the camera directly, keeping the current animation position
// when a new gesture interrupts a settling page.
func (d *slideDeck) dragBy(delta float32, now time.Time, page func(int) sceneRequest) {
	camera := d.position(now)
	if !d.dragging {
		// Rapid keyboard/button paging can leave the requested page several
		// slots ahead of the camera. Start from the page actually on screen,
		// otherwise clamping to the requested page would jump the pixels.
		var nearest *slideScene
		for i := range d.layers {
			layer := &d.layers[i]
			if layer.request.key.category == d.last.key.category &&
				(nearest == nil || abs(layer.slot-camera) < abs(nearest.slot-camera)) {
				nearest = layer
			}
		}
		if nearest != nil {
			d.to = nearest.slot
			d.targetID = nearest.id
			d.last = nearest.request
		}
	}
	d.dragging = true
	d.from = max(d.to-1, min(d.to+1, camera+delta/d.last.width))
	direction := 1
	if d.from < d.to {
		direction = -1
	}
	slot := d.to + float32(direction)
	request := page(d.last.key.page + direction)
	for _, layer := range d.layers {
		if layer.slot == slot && sameScene(layer.request, request) {
			return
		}
	}
	kept := d.layers[:0]
	for _, layer := range d.layers {
		if layer.id == d.targetID || layer.slot != slot && abs(layer.slot-d.from) < 1 {
			kept = append(kept, layer)
		}
	}
	d.layers = kept
	d.nextID++
	request.apps = append([]launcher.App(nil), request.apps...)
	d.layers = append(d.layers, slideScene{id: d.nextID, slot: slot, request: request})
}

func (d *slideDeck) frames(now time.Time) ([]sceneFrame, time.Duration) {
	camera := d.position(now)
	remaining := contentSlide.Duration - now.Sub(d.started)
	moving := d.from != d.to && remaining > 0
	if d.dragging {
		moving = true
	}
	if !moving {
		kept := d.layers[:0]
		for _, layer := range d.layers {
			if layer.id == d.targetID {
				kept = append(kept, layer)
			}
		}
		d.layers = kept
		d.from = d.to
	}
	frames := make([]sceneFrame, 0, 3)
	// Outgoing scenes paint first; the only interactive scene paints last.
	var current *sceneFrame
	for _, layer := range d.layers {
		frame := sceneFrame{scene: layer, offset: layer.slot - camera, target: layer.id == d.targetID}
		if frame.target {
			current = &frame
		} else if frame.offset > -1 && frame.offset < 1 {
			frames = append(frames, frame)
		}
	}
	if current != nil {
		frames = append(frames, *current)
	}
	if !moving || d.dragging {
		return frames, 0
	}
	return frames, remaining
}

func (v *Launchpad) slideFrames(c *ui.Context, width, height, size float32, loaded bool) []sceneFrame {
	from := v.page * v.pageSize
	to := min(len(v.filtered), from+v.pageSize)
	empty := ""
	if !loaded {
		empty = "native.loading"
	} else if len(v.filtered) == 0 {
		empty = "native.emptyCategory"
		if v.query != "" {
			empty = "native.emptySearch"
		}
	}
	v.mu.Lock()
	revision := v.iconEpoch
	v.mu.Unlock()
	request := sceneRequest{key: sceneKey{v.category, v.page}, apps: v.filtered[from:to], emptyKey: empty, query: v.query, width: width, height: height, size: size, capacity: v.pageSize, pageCount: v.pageCount, revision: revision, loaded: loaded}
	direction := v.pageDirection
	if v.categoryDirection != 0 {
		direction = v.categoryDirection
	}
	v.slides.sync(request, direction, c.Now(), c.Preferences().ReduceMotion, v.slideReset)
	if v.scroll.active && !v.slides.dragging {
		v.scroll = pageScroll{axis: -1, phased: v.scroll.phased, last: v.scroll.last}
	}
	if v.scroll.active && !v.scroll.phased {
		if c.Now().Sub(v.scroll.last) >= swipeIdle {
			v.finishPageScroll(c.Now(), false)
		} else {
			c.After(swipeIdle - c.Now().Sub(v.scroll.last))
		}
	}
	v.categoryDirection = 0
	v.pageDirection = 0
	v.slideReset = false
	frames, wake := v.slides.frames(c.Now())
	if v.slides.dragging {
		for i := range frames {
			frames[i].target = false
		}
		if c.Preferences().ReduceMotion {
			for _, layer := range v.slides.layers {
				if layer.id == v.slides.targetID {
					return []sceneFrame{{scene: layer}}
				}
			}
		}
	}
	if wake > 0 {
		c.AnimationFrame()
		c.After(wake)
	}
	return frames
}
