package bgjob_test

import (
	"bytes"
	"log"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/eagle-point/service-portal/internal/bgjob"
)

func TestSafe_RecoversFromPanic_AndLogs(t *testing.T) {
	// Redirect the stdlib logger to a buffer so we can check the panic line
	// is actually written (observability matters more than the recovery
	// itself — a silent recovery is worse than a visible one).
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	// The call must NOT re-raise; if recover() had missed, the test process
	// would abort here rather than reaching the assertions below.
	bgjob.Safe("test-worker", func() {
		panic("boom")
	})

	logged := buf.String()
	assert.Contains(t, logged, `bgjob "test-worker"`,
		"panic log must tag the worker name for diagnosis")
	assert.Contains(t, logged, "recovered panic: boom",
		"panic value must be included in the log line")
	assert.Contains(t, logged, "goroutine",
		"the log must include a stack trace so the panic is diagnosable")
}

func TestSafe_HappyPath_RunsFnAndReturnsNormally(t *testing.T) {
	log.SetOutput(new(bytes.Buffer))
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	called := false
	bgjob.Safe("noop", func() { called = true })
	assert.True(t, called, "Safe must invoke fn on the happy path")
}

func TestSafe_ConcurrentPanics_IsolatedPerCall(t *testing.T) {
	// Two Safe() calls from different goroutines — one panics, one doesn't —
	// must not interfere with each other.
	log.SetOutput(new(bytes.Buffer))
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	done := make(chan bool, 2)
	go func() {
		bgjob.Safe("panicky", func() { panic("async-boom") })
		done <- true
	}()
	go func() {
		bgjob.Safe("calm", func() {})
		done <- true
	}()
	<-done
	<-done
	// Reaching here without the test runner killing us means both recovered.
}
