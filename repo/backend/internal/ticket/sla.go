package ticket

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/eagle-point/service-portal/internal/bgjob"
	"github.com/eagle-point/service-portal/internal/models"
)

// SLAEngineInterval is how often the engine sweeps for breaches.
// Exposed as a var so tests can override it.
var SLAEngineInterval = 60 * time.Second

// StartSLAEngine launches a background goroutine that flips sla_breached = 1
// on tickets whose sla_deadline has passed and that are not yet in a final state.
// The goroutine exits when ctx is cancelled.
//
// notify is optional (may be nil). When provided, a breach notification is sent
// to the ticket owner for each newly-breached ticket.
func StartSLAEngine(ctx context.Context, db *sql.DB, notify DispatchFunc) {
	ticker := time.NewTicker(SLAEngineInterval)
	defer ticker.Stop()

	bgjob.Safe("sla-initial-sweep", func() {
		if err := SweepBreachesOnceWithNotify(ctx, db, notify); err != nil {
			log.Printf("sla: initial sweep failed: %v", err)
		}
	})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Safe() prevents a single bad row or misbehaving notify closure
			// from killing the goroutine and halting all future sweeps.
			bgjob.Safe("sla-sweep", func() {
				if err := SweepBreachesOnceWithNotify(ctx, db, notify); err != nil {
					log.Printf("sla: sweep failed: %v", err)
				}
			})
		}
	}
}

// SweepBreachesOnce runs a single SLA sweep without notifications.
// Kept as a thin wrapper for older callers and unit tests.
func SweepBreachesOnce(ctx context.Context, db *sql.DB) error {
	return SweepBreachesOnceWithNotify(ctx, db, nil)
}

// SweepBreachesOnceWithNotify runs a single SLA sweep and optionally dispatches
// a breach notification per newly-breached ticket.
//
// Implementation note: we first SELECT the tickets that are about to breach so
// we have their IDs + owners, then UPDATE and dispatch. This avoids duplicate
// notifications on subsequent sweeps because the UPDATE flips sla_breached=1.
func SweepBreachesOnceWithNotify(ctx context.Context, db *sql.DB, notify DispatchFunc) error {
	rows, err := db.QueryContext(ctx,
		`SELECT id, user_id FROM tickets
		 WHERE sla_deadline IS NOT NULL
		   AND sla_deadline < NOW()
		   AND sla_breached = 0
		   AND status NOT IN ('Completed', 'Closed', 'Cancelled')`)
	if err != nil {
		return err
	}
	type breach struct {
		ticketID uint64
		userID   uint64
	}
	var toBreach []breach
	for rows.Next() {
		var b breach
		if err := rows.Scan(&b.ticketID, &b.userID); err != nil {
			rows.Close()
			return err
		}
		toBreach = append(toBreach, b)
	}
	rows.Close()

	if len(toBreach) == 0 {
		return nil
	}

	// Flip the flag in bulk.
	if _, err := db.ExecContext(ctx,
		`UPDATE tickets
		 SET sla_breached = 1
		 WHERE sla_deadline IS NOT NULL
		   AND sla_deadline < NOW()
		   AND sla_breached = 0
		   AND status NOT IN ('Completed', 'Closed', 'Cancelled')`); err != nil {
		return err
	}

	if notify == nil {
		return nil
	}
	for _, b := range toBreach {
		_ = notify(ctx, b.userID, models.NotifSLABreach, map[string]any{
			"TicketID": b.ticketID,
		})
	}
	return nil
}
