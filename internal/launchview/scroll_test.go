package launchview

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func swipePad() (*Launchpad, time.Time) {
	now := time.Unix(100, 0)
	apps := fixture(30).state.Apps
	v := &Launchpad{pageCount: 3, pageSize: 10, filtered: apps}
	v.state.Preferences.LoopPages = true
	v.slides.reset(slideRequest("", 0, apps[:10]), now)
	return v, now
}

func TestTrackpadFollowsAndSnapsBack(t *testing.T) {
	v, now := swipePad()
	send := func(dx float32, phase scrollPhase, delay time.Duration) {
		now = now.Add(delay)
		v.handlePageScrollPhase(ui.InputEvent{Kind: ui.InputScroll, DX: dx, Precise: true}, now, phase)
	}
	send(80, scrollBegan, 20*time.Millisecond)
	closeOffset(t, v.slides.position(now), 80.0/900)
	if v.page != 0 {
		t.Fatal("drag committed before release")
	}
	send(-60, scrollChanged, 20*time.Millisecond)
	closeOffset(t, v.slides.position(now), 20.0/900)
	send(0, scrollEnded, 120*time.Millisecond)
	if v.page != 0 || v.slides.dragging {
		t.Fatal("short drag should snap back")
	}
	closeOffset(t, v.slides.position(now), 20.0/900)
	closeOffset(t, v.slides.position(now.Add(contentSlide.Duration)), 0)
}

func TestTrackpadReleaseVelocityWrapAndCancellation(t *testing.T) {
	v, now := swipePad()
	send := func(dx float32, phase scrollPhase, delay time.Duration) {
		now = now.Add(delay)
		v.handlePageScrollPhase(ui.InputEvent{Kind: ui.InputScroll, DX: dx, Precise: true}, now, phase)
	}
	send(-250, scrollBegan, 20*time.Millisecond)
	send(0, scrollEnded, 100*time.Millisecond)
	if v.page != 2 {
		t.Fatal("reverse drag did not wrap to last page")
	}
	now = now.Add(contentSlide.Duration)
	v.slides.frames(now)
	send(90, scrollBegan, 20*time.Millisecond)
	before := v.slides.position(now)
	send(0, scrollEnded, 10*time.Millisecond)
	if v.page != 0 {
		t.Fatal("fast short swipe did not wrap forward")
	}
	closeOffset(t, v.slides.position(now), before)
	now = now.Add(contentSlide.Duration)
	v.slides.frames(now)
	send(300, scrollBegan, 20*time.Millisecond)
	send(0, scrollCancelled, 10*time.Millisecond)
	if v.page != 0 {
		t.Fatal("cancelled gesture turned a page")
	}
	for range 40 {
		now = now.Add(10 * time.Millisecond)
		v.handlePageScrollPhase(ui.InputEvent{Kind: ui.InputScroll, DX: 40, Precise: true, Momentum: true}, now, scrollUnphased)
	}
	if v.page != 0 {
		t.Fatal("momentum turned additional pages")
	}
}

func TestTrackpadLocksVerticalGestureAndAllowsImmediateNewGesture(t *testing.T) {
	v, now := swipePad()
	v.handlePageScrollPhase(ui.InputEvent{Kind: ui.InputScroll, DX: 1, DY: 40, Precise: true}, now, scrollBegan)
	v.handlePageScrollPhase(ui.InputEvent{Kind: ui.InputScroll, DX: 300, Precise: true}, now.Add(10*time.Millisecond), scrollChanged)
	if v.slides.dragging {
		t.Fatal("vertical gesture changed into horizontal paging")
	}
	v.handlePageScrollPhase(ui.InputEvent{Kind: ui.InputScroll, Precise: true}, now.Add(20*time.Millisecond), scrollEnded)
	v.handlePageScrollPhase(ui.InputEvent{Kind: ui.InputScroll, DX: 250, Precise: true}, now.Add(30*time.Millisecond), scrollBegan)
	v.handlePageScrollPhase(ui.InputEvent{Kind: ui.InputScroll, Precise: true}, now.Add(40*time.Millisecond), scrollEnded)
	if v.page != 1 {
		t.Fatal("new native gesture was blocked by a cooldown")
	}
}

func TestWheelAndSinglePage(t *testing.T) {
	v := &Launchpad{pageCount: 3}
	now := time.Now()
	ev := ui.InputEvent{Kind: ui.InputScroll, DY: 40}
	v.handlePageScroll(ev, now)
	v.handlePageScroll(ev, now.Add(100*time.Millisecond))
	if v.page != 1 {
		t.Fatal("wheel cooldown did not suppress duplicate paging")
	}
	v.handlePageScroll(ev, now.Add(400*time.Millisecond))
	if v.page != 2 {
		t.Fatal("wheel did not page after cooldown")
	}
	v.pageCount, v.page = 1, 0
	if v.handlePageScroll(ev, now.Add(time.Second)) || v.page != 0 {
		t.Fatal("single page should ignore scrolling")
	}
}

func TestPagingControlsWrapWithForwardAnimation(t *testing.T) {
	v := testPad(fixture(35), Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	first, _ := tt.Find("工具 000")
	for range v.pageCount - 1 {
		tt.Key(0, ui.KeyPageDown)
		tt.Advance(250 * time.Millisecond)
	}
	if err := tt.Click(v.t("native.nextPage")); err != nil {
		t.Fatal(err)
	}
	if v.page != 0 {
		t.Fatal("next button did not wrap to first page")
	}
	tt.Advance(80 * time.Millisecond)
	if moving, ok := tt.Find("工具 000"); !ok || moving.X <= first.X {
		t.Fatalf("wrapped first page should enter from the right: %v found=%v", moving, ok)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click(v.t("native.previousPage")); err != nil {
		t.Fatal(err)
	}
	if v.page != v.pageCount-1 {
		t.Fatal("previous button did not wrap to last page")
	}
	tt.Advance(250 * time.Millisecond)
	tt.Scroll(450, 320, 250, 0)
	if v.page != v.pageCount-1 || !v.slides.dragging {
		t.Fatal("trackpad should drag before committing")
	}
	tt.Advance(swipeIdle)
	if v.page != 0 {
		t.Fatal("horizontal trackpad input did not reach the pager")
	}
}

func TestTrackpadRenderedPixelsFollowAndSearchCancels(t *testing.T) {
	v := testPad(fixture(70), Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	tt.Scroll(450, 320, 80, 0)
	frames, _ := v.slides.frames(tt.Now())
	if len(frames) != 2 {
		t.Fatalf("drag should preview the adjacent page: %d", len(frames))
	}
	for _, frame := range frames {
		if frame.scene.request.key.page == 0 {
			closeOffset(t, frame.offset, -80.0/900)
		}
	}
	tt.Advance(16 * time.Millisecond)
	tt.Scroll(450, 320, -60, 0)
	closeOffset(t, v.slides.position(tt.Now()), 20.0/900)
	tt.Type("工具 001")
	if v.scroll.active || v.slides.dragging || v.page != 0 {
		t.Fatal("search did not cancel the drag")
	}
	tt.Advance(time.Second)
	if v.page != 0 || len(v.filtered) != 1 {
		t.Fatal("cancelled drag changed search results later")
	}
}

func TestTrackpadCannotLaunchWhileDraggingAndHonorsReducedMotion(t *testing.T) {
	f := fixture(70)
	v := testPad(f, Callbacks{})
	defer v.Close()
	tt := ui.NewTester(v.View, 900, 650)
	tile, _ := tt.Find("工具 000")
	tt.Scroll(450, 320, 40, 0)
	tt.ClickAt(tile.X+tile.W/2-40, tile.Y-45)
	tt.Key(0, ui.KeyEnter)
	select {
	case id := <-f.launched:
		t.Fatalf("drag launched application %s", id)
	default:
	}
	tt.Type("工具 000")
	v.query = ""
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	tt.Advance(200 * time.Millisecond)
	tt.Scroll(450, 320, 250, 0)
	if !v.slides.dragging {
		t.Fatal("reduced motion should still recognize swipes")
	}
	tt.Advance(swipeIdle)
	frames, _ := v.slides.frames(tt.Now())
	if v.page != 1 || len(frames) != 1 || frames[0].offset != 0 {
		t.Fatal("reduced-motion swipe did not settle immediately")
	}
}

func TestRapidDragReversalsKeepCameraAndBoundHistory(t *testing.T) {
	v, now := swipePad()
	v.slides.sync(v.pageRequest(1), 1, now, false, false)
	v.page = 1
	now = now.Add(50 * time.Millisecond)
	before := v.slides.position(now)
	v.slides.dragBy(9, now, v.pageRequest)
	closeOffset(t, v.slides.position(now), before+.01)
	for i := 0; i < 1000; i++ {
		delta := float32(20)
		if i%2 == 0 {
			delta = -20
		}
		v.slides.dragBy(delta, now, v.pageRequest)
		frames, _ := v.slides.frames(now)
		if len(v.slides.layers) > 3 || len(frames) > 3 {
			t.Fatal("drag accumulated historical scenes")
		}
	}
}

func TestTrackpadInterruptsQueuedPagesWithoutJumping(t *testing.T) {
	v, now := swipePad()
	v.slides.sync(v.pageRequest(1), 1, now, false, false)
	now = now.Add(10 * time.Millisecond)
	v.slides.sync(v.pageRequest(2), 1, now, false, false)
	v.page = 2
	now = now.Add(10 * time.Millisecond)
	before := v.slides.position(now)
	v.handlePageScrollPhase(ui.InputEvent{Kind: ui.InputScroll, DX: 9, Precise: true}, now, scrollBegan)
	closeOffset(t, v.slides.position(now), before+.01)
	if v.page != v.slides.last.key.page || v.page == 2 {
		t.Fatal("drag should take over the visible page rather than the queued destination")
	}
}
