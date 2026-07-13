package core

import (
	"sync"
	"sync/atomic"
	"testing"
)

// TestClaim_NeverOverlaps is the regression guard for the check-then-act race in
// the run gate: two dispatches (e.g. a Telegram worker and the scheduler
// goroutine) both observing an idle engine and both starting a run. No two
// claims may hold the slot at once. Run with -race.
func TestClaim_NeverOverlaps(t *testing.T) {
	e := &Engine{}

	const goroutines = 100
	var (
		wg      sync.WaitGroup
		start   = make(chan struct{})
		active  atomic.Int32 // how many claims currently hold the slot
		maxSeen atomic.Int32
		wins    atomic.Int32
	)

	for range goroutines {
		wg.Go(func() {
			run := &activeRun{cancel: func() {}, replyInbox: make(chan string)}
			<-start // release all at once to maximize contention
			release, _, ok := e.claim(run)
			if !ok {
				return
			}
			wins.Add(1)
			n := active.Add(1)
			for {
				m := maxSeen.Load()
				if n <= m || maxSeen.CompareAndSwap(m, n) {
					break
				}
			}
			active.Add(-1)
			release()
		})
	}

	close(start)
	wg.Wait()

	if got := maxSeen.Load(); got > 1 {
		t.Fatalf("two runs held the slot at once (max concurrent = %d): the gate is not atomic", got)
	}
	if wins.Load() == 0 {
		t.Fatal("no claim ever succeeded")
	}
	if e.active != nil {
		t.Fatalf("engine left busy after all runs released: %+v", e.active)
	}
}

// TestClaim_RejectsSecondWhileActive asserts the busy path: while one run holds
// the slot, a concurrent claim must fail and hand back the active run.
func TestClaim_RejectsSecondWhileActive(t *testing.T) {
	e := &Engine{}

	first := &activeRun{cancel: func() {}, replyInbox: make(chan string)}
	release, _, ok := e.claim(first)
	if !ok {
		t.Fatal("first claim should succeed on an idle engine")
	}

	second := &activeRun{cancel: func() {}, replyInbox: make(chan string)}
	_, existing, ok := e.claim(second)
	if ok {
		t.Fatal("second claim should fail while a run is active")
	}
	if existing != first {
		t.Fatal("busy claim should return the in-flight run")
	}

	release()
	if _, _, ok := e.claim(second); !ok {
		t.Fatal("claim should succeed once the slot is released")
	}
}
