// Package bgjob provides a single helper for running background-worker tick
// functions inside a panic barrier.
//
// Why this exists: each of the three background workers the server starts
// (ticket SLA engine, privacy export generator, privacy deletion processor)
// runs in its own goroutine driven by a time.Ticker. If the tick function
// panics — because of a nil DB row, a misbehaving notify hook, or any data-
// dependent edge case — the default behaviour kills just that goroutine
// while leaving the rest of the process healthy. From the operator's
// perspective the worker silently stops: exports never generate, deletions
// never fire, SLAs never breach. There is no obvious error signal because
// other HTTP handlers keep responding.
//
// Safe wraps a single tick in recover() + logging so a panic is observable
// (stack goes to the log) and recoverable (the next tick still fires).
package bgjob

import (
	"log"
	"runtime/debug"
)

// Safe runs fn and recovers from any panic. Name is included in the recovery
// log line so multiple workers' panics can be distinguished.
//
// Call this inside the select branch that receives from the ticker, NOT
// around the outer for-loop — we want the loop to keep ticking even if one
// tick panics.
func Safe(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("bgjob %q: recovered panic: %v\n%s", name, r, debug.Stack())
		}
	}()
	fn()
}
