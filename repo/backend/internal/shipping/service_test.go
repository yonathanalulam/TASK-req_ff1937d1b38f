package shipping_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eagle-point/service-portal/internal/shipping"
	"github.com/eagle-point/service-portal/internal/testutil"
)

// ─── Integration: regions ─────────────────────────────────────────────────────

func TestShipping_CreateAndListRegions(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "shipping_templates", "shipping_regions")

	svc := shipping.NewService(db)

	region, err := svc.CreateRegion(context.Background(), shipping.CreateRegionInput{
		Name:       "Northeast",
		CutoffTime: "16:00:00",
		Timezone:   "America/New_York",
	})
	require.NoError(t, err)
	assert.Equal(t, "Northeast", region.Name)
	assert.Equal(t, "16:00:00", region.CutoffTime)
	assert.NotZero(t, region.ID)

	regions, err := svc.ListRegions(context.Background())
	require.NoError(t, err)
	assert.Len(t, regions, 1)
	assert.Equal(t, "Northeast", regions[0].Name)
}

func TestShipping_CreateRegion_DefaultsApplied(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "shipping_templates", "shipping_regions")

	svc := shipping.NewService(db)

	// Omit cutoff_time and timezone → service applies defaults
	region, err := svc.CreateRegion(context.Background(), shipping.CreateRegionInput{
		Name: "West",
	})
	require.NoError(t, err)
	assert.Equal(t, "17:00:00", region.CutoffTime)
	assert.Equal(t, "America/New_York", region.Timezone)
}

// ─── Integration: templates ───────────────────────────────────────────────────

func TestShipping_CreateAndListTemplates(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "shipping_templates", "shipping_regions")

	svc := shipping.NewService(db)
	region, _ := svc.CreateRegion(context.Background(), shipping.CreateRegionInput{Name: "South"})

	tmpl, err := svc.CreateTemplate(context.Background(), shipping.CreateTemplateInput{
		RegionID:       region.ID,
		DeliveryMethod: "courier",
		MinWeightKg:    0,
		MaxWeightKg:    20,
		MinQuantity:    1,
		MaxQuantity:    10,
		FeeAmount:      12.50,
		Currency:       "USD",
		LeadTimeHours:  24,
		WindowHours:    4,
	})
	require.NoError(t, err)
	assert.Equal(t, "courier", tmpl.DeliveryMethod)
	assert.Equal(t, 12.50, tmpl.FeeAmount)

	templates, err := svc.ListTemplates(context.Background(), region.ID)
	require.NoError(t, err)
	assert.Len(t, templates, 1)
}

// ─── Integration: estimate — pickup ──────────────────────────────────────────

func TestEstimate_Pickup_FeeZeroNoWindow(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "shipping_templates", "shipping_regions")

	svc := shipping.NewService(db)
	region, _ := svc.CreateRegion(context.Background(), shipping.CreateRegionInput{Name: "Local"})

	result, err := svc.Estimate(context.Background(), shipping.EstimateInput{
		RegionID:       region.ID,
		WeightKg:       5.0,
		Quantity:       2,
		DeliveryMethod: "pickup",
		RequestedAt:    time.Now().UTC(),
	})
	require.NoError(t, err)
	assert.Equal(t, 0.0, result.Fee)
	assert.Equal(t, "USD", result.Currency)
	assert.Empty(t, result.EstimatedArrivalWindow)
}

// ─── Integration: estimate — courier ─────────────────────────────────────────

func TestEstimate_Courier_FeeAndWindow(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "shipping_templates", "shipping_regions")

	svc := shipping.NewService(db)

	region, err := svc.CreateRegion(context.Background(), shipping.CreateRegionInput{
		Name:       "Metro",
		CutoffTime: "17:00:00",
		Timezone:   "UTC",
	})
	require.NoError(t, err)

	_, err = svc.CreateTemplate(context.Background(), shipping.CreateTemplateInput{
		RegionID:       region.ID,
		DeliveryMethod: "courier",
		MinWeightKg:    0,
		MaxWeightKg:    50,
		MinQuantity:    1,
		MaxQuantity:    100,
		FeeAmount:      15.00,
		Currency:       "USD",
		LeadTimeHours:  24,
		WindowHours:    4,
	})
	require.NoError(t, err)

	// Request well before cutoff
	requestedAt := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	result, err := svc.Estimate(context.Background(), shipping.EstimateInput{
		RegionID:       region.ID,
		WeightKg:       5.0,
		Quantity:       2,
		DeliveryMethod: "courier",
		RequestedAt:    requestedAt,
	})
	require.NoError(t, err)
	assert.Equal(t, 15.00, result.Fee)
	assert.Equal(t, "USD", result.Currency)
	assert.NotEmpty(t, result.EstimatedArrivalWindow)
	// lead=24h from 17:00 on 4/15 → arrives 4/16 at 17:00–21:00 PM
	assert.Equal(t, "Arrives 4/16/2026, 5:00–9:00 PM", result.EstimatedArrivalWindow)
}

func TestEstimate_Courier_NoMatchingTemplate_ReturnsErrNoTemplate(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "shipping_templates", "shipping_regions")

	svc := shipping.NewService(db)
	region, _ := svc.CreateRegion(context.Background(), shipping.CreateRegionInput{Name: "Empty"})

	_, err := svc.Estimate(context.Background(), shipping.EstimateInput{
		RegionID:       region.ID,
		WeightKg:       5.0,
		Quantity:       1,
		DeliveryMethod: "courier",
		RequestedAt:    time.Now().UTC(),
	})
	assert.ErrorIs(t, err, shipping.ErrNoTemplate)
}

func TestEstimate_Courier_RegionNotFound(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "shipping_templates", "shipping_regions")

	svc := shipping.NewService(db)
	_, err := svc.Estimate(context.Background(), shipping.EstimateInput{
		RegionID:       99999,
		DeliveryMethod: "courier",
		RequestedAt:    time.Now().UTC(),
	})
	assert.ErrorIs(t, err, shipping.ErrNotFound)
}

func TestEstimate_Courier_CheapestTemplateChosen(t *testing.T) {
	db := testutil.DBOrSkip(t)
	testutil.TruncateTables(t, db, "shipping_templates", "shipping_regions")

	svc := shipping.NewService(db)
	region, _ := svc.CreateRegion(context.Background(), shipping.CreateRegionInput{
		Name: "Multi", CutoffTime: "17:00:00", Timezone: "UTC",
	})

	// Two overlapping courier brackets; cheaper one should win
	svc.CreateTemplate(context.Background(), shipping.CreateTemplateInput{
		RegionID:       region.ID,
		DeliveryMethod: "courier",
		MinWeightKg:    0, MaxWeightKg: 20,
		MinQuantity:    1, MaxQuantity: 10,
		FeeAmount: 25.00, Currency: "USD",
		LeadTimeHours: 48, WindowHours: 4,
	})
	svc.CreateTemplate(context.Background(), shipping.CreateTemplateInput{
		RegionID:       region.ID,
		DeliveryMethod: "courier",
		MinWeightKg:    0, MaxWeightKg: 20,
		MinQuantity:    1, MaxQuantity: 10,
		FeeAmount: 10.00, Currency: "USD",
		LeadTimeHours: 24, WindowHours: 4,
	})

	requestedAt := time.Date(2026, 4, 15, 10, 0, 0, 0, time.UTC)
	result, err := svc.Estimate(context.Background(), shipping.EstimateInput{
		RegionID:       region.ID,
		WeightKg:       5.0,
		Quantity:       2,
		DeliveryMethod: "courier",
		RequestedAt:    requestedAt,
	})
	require.NoError(t, err)
	assert.Equal(t, 10.00, result.Fee)
}
