package shipping

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata" // embed timezone data for portability
)

// ComputeETA calculates an estimated arrival window string.
//
//   - timezone:       IANA timezone name (e.g. "America/New_York")
//   - cutoffTime:     daily order cutoff in "HH:MM:SS" format
//   - leadTimeHours:  hours from cutoff to earliest delivery
//   - windowHours:    duration of the delivery window in hours
//   - requestedAt:    moment the request is made (UTC or any zone)
//
// Returns a string like "Arrives 3/28/2026, 2:00–6:00 PM".
func ComputeETA(timezone, cutoffTime string, leadTimeHours, windowHours int, requestedAt time.Time) string {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc = time.UTC
	}

	localReq := requestedAt.In(loc)
	cutoff := buildCutoff(localReq, cutoffTime, loc)

	var base time.Time
	if localReq.Before(cutoff) {
		// Order placed before today's cutoff → process today
		base = cutoff
	} else {
		// Order placed at or after cutoff → process tomorrow
		base = cutoff.AddDate(0, 0, 1)
	}

	start := base.Add(time.Duration(leadTimeHours) * time.Hour)
	end := start.Add(time.Duration(windowHours) * time.Hour)

	return formatWindow(start, end)
}

// FindMatchingTemplate returns the first template whose weight and quantity
// brackets contain the given values, or nil if none match.
func FindMatchingTemplate(templates []*ShippingTemplateParams, method string, weightKg float64, quantity int) *ShippingTemplateParams {
	for _, t := range templates {
		if t.DeliveryMethod != method {
			continue
		}
		if weightKg < t.MinWeightKg || weightKg > t.MaxWeightKg {
			continue
		}
		if quantity < t.MinQuantity || quantity > t.MaxQuantity {
			continue
		}
		return t
	}
	return nil
}

// ShippingTemplateParams is a value-type copy of a template used for pure-function testing.
type ShippingTemplateParams struct {
	DeliveryMethod string
	MinWeightKg    float64
	MaxWeightKg    float64
	MinQuantity    int
	MaxQuantity    int
	FeeAmount      float64
	Currency       string
	LeadTimeHours  int
	WindowHours    int
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func buildCutoff(ref time.Time, cutoffStr string, loc *time.Location) time.Time {
	parts := strings.Split(cutoffStr, ":")
	h, m := 17, 0 // safe default
	if len(parts) >= 2 {
		h, _ = strconv.Atoi(parts[0])
		m, _ = strconv.Atoi(parts[1])
	}
	return time.Date(ref.Year(), ref.Month(), ref.Day(), h, m, 0, 0, loc)
}

func formatWindow(start, end time.Time) string {
	date := start.Format("1/2/2006")
	startStr := formatHM(start)
	endStr := formatHM(end)
	period := "AM"
	if end.Hour() >= 12 {
		period = "PM"
	}
	return fmt.Sprintf("Arrives %s, %s–%s %s", date, startStr, endStr, period)
}

func formatHM(t time.Time) string {
	h := t.Hour()
	m := t.Minute()
	if h == 0 {
		h = 12
	} else if h > 12 {
		h -= 12
	}
	if m == 0 {
		return fmt.Sprintf("%d:00", h)
	}
	return fmt.Sprintf("%d:%02d", h, m)
}
