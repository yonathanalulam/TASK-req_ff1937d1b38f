package shipping

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// ─── ComputeETA ───────────────────────────────────────────────────────────────

func TestComputeETA_BeforeCutoff(t *testing.T) {
	// Request at noon UTC, cutoff 17:00 UTC → base = today's cutoff
	// lead=24h, window=4h → start = tomorrow 17:00, end = tomorrow 21:00 → PM
	requestedAt := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	result := ComputeETA("UTC", "17:00:00", 24, 4, requestedAt)
	assert.Equal(t, "Arrives 4/16/2026, 5:00–9:00 PM", result)
}

func TestComputeETA_AfterCutoff(t *testing.T) {
	// Request at 18:00 UTC (past 17:00 cutoff) → base = tomorrow's cutoff
	// lead=24h, window=4h → start = day-after-tomorrow 17:00, end 21:00 → PM
	requestedAt := time.Date(2026, 4, 15, 18, 0, 0, 0, time.UTC)
	result := ComputeETA("UTC", "17:00:00", 24, 4, requestedAt)
	assert.Equal(t, "Arrives 4/17/2026, 5:00–9:00 PM", result)
}

func TestComputeETA_ExactlyAtCutoff(t *testing.T) {
	// Exactly at cutoff is NOT before → next-day processing
	requestedAt := time.Date(2026, 4, 15, 17, 0, 0, 0, time.UTC)
	result := ComputeETA("UTC", "17:00:00", 24, 4, requestedAt)
	assert.Equal(t, "Arrives 4/17/2026, 5:00–9:00 PM", result)
}

func TestComputeETA_AMWindow(t *testing.T) {
	// Cutoff 23:00, lead=10h → start next day at 09:00, window=2h → end 11:00 → AM
	requestedAt := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	result := ComputeETA("UTC", "23:00:00", 10, 2, requestedAt)
	assert.Equal(t, "Arrives 4/16/2026, 9:00–11:00 AM", result)
}

func TestComputeETA_SameDayWindow(t *testing.T) {
	// Request at 06:00, cutoff 17:00, lead=2h, window=2h
	// base = today 17:00, start = 19:00, end = 21:00 → PM, arrives same date
	requestedAt := time.Date(2026, 4, 15, 6, 0, 0, 0, time.UTC)
	result := ComputeETA("UTC", "17:00:00", 2, 2, requestedAt)
	assert.Equal(t, "Arrives 4/15/2026, 7:00–9:00 PM", result)
}

func TestComputeETA_InvalidTimezone_FallsBackToUTC(t *testing.T) {
	requestedAt := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	// Should not panic; falls back to UTC
	result := ComputeETA("Invalid/Zone", "17:00:00", 24, 4, requestedAt)
	assert.Equal(t, "Arrives 4/16/2026, 5:00–9:00 PM", result)
}

func TestComputeETA_MidnightHour(t *testing.T) {
	// start = midnight (hour 0) → formatted as "12:00"
	// cutoff 14:00, lead=10h → start 00:00 next day, window=2h → end 02:00 → AM
	requestedAt := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	result := ComputeETA("UTC", "14:00:00", 10, 2, requestedAt)
	assert.Equal(t, "Arrives 4/16/2026, 12:00–2:00 AM", result)
}

// ─── FindMatchingTemplate ─────────────────────────────────────────────────────

func TestFindMatchingTemplate_Matches(t *testing.T) {
	templates := []*ShippingTemplateParams{
		{
			DeliveryMethod: "courier",
			MinWeightKg:    0, MaxWeightKg: 10,
			MinQuantity: 1, MaxQuantity: 5,
			FeeAmount: 9.99, Currency: "USD",
		},
	}
	got := FindMatchingTemplate(templates, "courier", 5.0, 3)
	assert.NotNil(t, got)
	assert.Equal(t, 9.99, got.FeeAmount)
}

func TestFindMatchingTemplate_WeightTooHigh(t *testing.T) {
	templates := []*ShippingTemplateParams{
		{
			DeliveryMethod: "courier",
			MinWeightKg:    0, MaxWeightKg: 10,
			MinQuantity: 1, MaxQuantity: 5,
			FeeAmount: 9.99,
		},
	}
	got := FindMatchingTemplate(templates, "courier", 11.0, 3)
	assert.Nil(t, got)
}

func TestFindMatchingTemplate_QuantityTooHigh(t *testing.T) {
	templates := []*ShippingTemplateParams{
		{
			DeliveryMethod: "courier",
			MinWeightKg:    0, MaxWeightKg: 10,
			MinQuantity: 1, MaxQuantity: 5,
			FeeAmount: 9.99,
		},
	}
	got := FindMatchingTemplate(templates, "courier", 5.0, 6)
	assert.Nil(t, got)
}

func TestFindMatchingTemplate_WrongMethod(t *testing.T) {
	templates := []*ShippingTemplateParams{
		{
			DeliveryMethod: "courier",
			MinWeightKg:    0, MaxWeightKg: 10,
			MinQuantity: 1, MaxQuantity: 5,
			FeeAmount: 9.99,
		},
	}
	got := FindMatchingTemplate(templates, "pickup", 5.0, 3)
	assert.Nil(t, got)
}

func TestFindMatchingTemplate_BoundaryInclusive(t *testing.T) {
	templates := []*ShippingTemplateParams{
		{
			DeliveryMethod: "courier",
			MinWeightKg:    5, MaxWeightKg: 10,
			MinQuantity: 2, MaxQuantity: 4,
			FeeAmount: 7.50,
		},
	}
	// Exactly at min/max boundaries should match
	got := FindMatchingTemplate(templates, "courier", 5.0, 2)
	assert.NotNil(t, got)
	got = FindMatchingTemplate(templates, "courier", 10.0, 4)
	assert.NotNil(t, got)
}

func TestFindMatchingTemplate_Empty(t *testing.T) {
	got := FindMatchingTemplate(nil, "courier", 5.0, 3)
	assert.Nil(t, got)
}

func TestFindMatchingTemplate_FirstMatchReturned(t *testing.T) {
	templates := []*ShippingTemplateParams{
		{DeliveryMethod: "courier", MinWeightKg: 0, MaxWeightKg: 10, MinQuantity: 1, MaxQuantity: 10, FeeAmount: 12.00},
		{DeliveryMethod: "courier", MinWeightKg: 0, MaxWeightKg: 10, MinQuantity: 1, MaxQuantity: 10, FeeAmount: 5.00},
	}
	got := FindMatchingTemplate(templates, "courier", 5.0, 3)
	assert.NotNil(t, got)
	// Returns first match, not cheapest (DB layer handles cheapest via ORDER BY)
	assert.Equal(t, 12.00, got.FeeAmount)
}
