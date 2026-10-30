package main

import (
	"reflect"
	"testing"
)

func TestManagerCloseWaitsForSaveAndMergesQuit(t *testing.T) {
	var events []string
	var finish func(bool)
	closer := managerCloser{
		prepare: func(done func(bool)) { events = append(events, "prepare"); finish = done },
		destroy: func() { events = append(events, "destroy") },
		quit:    func() { events = append(events, "quit") },
	}
	closer.request(false)
	closer.request(false)
	closer.request(true)
	closer.request(true)
	if !reflect.DeepEqual(events, []string{"prepare"}) {
		t.Fatalf("closed before saving or started duplicate save barriers: %v", events)
	}
	finish(true)
	if !reflect.DeepEqual(events, []string{"prepare", "destroy", "quit"}) {
		t.Fatalf("quit must follow saved settings and window destruction: %v", events)
	}
}

func TestManagerCloseFailureKeepsWindowAndAllowsRetry(t *testing.T) {
	var events []string
	var finish func(bool)
	closer := managerCloser{
		prepare: func(done func(bool)) { events = append(events, "prepare"); finish = done },
		destroy: func() { events = append(events, "destroy") },
		quit:    func() { events = append(events, "quit") },
	}
	closer.request(true)
	finish(false)
	if !reflect.DeepEqual(events, []string{"prepare"}) {
		t.Fatalf("failed save destroyed settings or quit: %v", events)
	}
	// A later ordinary close must not inherit the cancelled quit request.
	closer.request(false)
	finish(true)
	if !reflect.DeepEqual(events, []string{"prepare", "prepare", "destroy"}) {
		t.Fatalf("retry failed or retained an obsolete quit request: %v", events)
	}
}
