package lakehouse

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
)

// lifecycleInterval is how often the worker re-runs RunLifecycle.
// Daily is matches the documented retention cadence (90 days / 18 months),
// while still being short enough that manual QA doesn't have to wait a week
// to see the worker make progress.
const lifecycleInterval = 24 * time.Hour

// StartLifecycleWorker runs RunLifecycle on a daily cadence until ctx is done.
// Intended to be launched once from router.New as a goroutine.
//
// The first sweep runs after a short startup grace period (1 minute) so the
// app isn't doing disk work while migrations/seeds are still settling in.
func StartLifecycleWorker(ctx context.Context, svc *Service, archiveDays, purgeDays int) {
	select {
	case <-time.After(1 * time.Minute):
	case <-ctx.Done():
		return
	}

	tick := time.NewTicker(lifecycleInterval)
	defer tick.Stop()

	runOnce := func() {
		res, err := svc.RunLifecycle(ctx, archiveDays, purgeDays)
		if err != nil {
			log.Error().Err(err).Msg("lakehouse lifecycle sweep failed")
			return
		}
		log.Info().
			Int("archived", res.Archived).
			Int("purged", res.Purged).
			Int("held", res.Held).
			Msg("lakehouse lifecycle sweep complete")
	}
	runOnce()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			runOnce()
		}
	}
}
