package shipping

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eagle-point/service-portal/internal/models"
)

// ErrNotFound is returned when the requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrNoTemplate is returned when no fee template matches the input parameters.
var ErrNoTemplate = errors.New("no shipping template found for given parameters")

// Service provides shipping region, template, and fee-estimation logic.
type Service struct {
	db *sql.DB
}

// NewService creates a shipping Service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// ─── Regions ─────────────────────────────────────────────────────────────────

// CreateRegionInput carries fields for a new shipping region.
type CreateRegionInput struct {
	Name       string
	CutoffTime string // "HH:MM:SS"
	Timezone   string
}

// ListRegions returns all shipping regions.
func (s *Service) ListRegions(ctx context.Context) ([]*models.ShippingRegion, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, cutoff_time, timezone, created_at, updated_at
		 FROM shipping_regions ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("shipping.ListRegions: %w", err)
	}
	defer rows.Close()

	var regions []*models.ShippingRegion
	for rows.Next() {
		r := &models.ShippingRegion{}
		if err := rows.Scan(&r.ID, &r.Name, &r.CutoffTime, &r.Timezone,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("shipping.ListRegions: scan: %w", err)
		}
		regions = append(regions, r)
	}
	if regions == nil {
		regions = []*models.ShippingRegion{}
	}
	return regions, rows.Err()
}

// CreateRegion adds a new shipping region.
func (s *Service) CreateRegion(ctx context.Context, in CreateRegionInput) (*models.ShippingRegion, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, errors.New("name is required")
	}
	if in.CutoffTime == "" {
		in.CutoffTime = "17:00:00"
	}
	if in.Timezone == "" {
		in.Timezone = "America/New_York"
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO shipping_regions (name, cutoff_time, timezone) VALUES (?, ?, ?)`,
		in.Name, in.CutoffTime, in.Timezone,
	)
	if err != nil {
		return nil, fmt.Errorf("shipping.CreateRegion: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.getRegionByID(ctx, uint64(id))
}

// ─── Templates ───────────────────────────────────────────────────────────────

// CreateTemplateInput carries fields for a new shipping template.
type CreateTemplateInput struct {
	RegionID       uint64
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

// ListTemplates returns all templates, optionally filtered by region.
func (s *Service) ListTemplates(ctx context.Context, regionID uint64) ([]*models.ShippingTemplate, error) {
	q := `SELECT id, region_id, delivery_method, min_weight_kg, max_weight_kg,
		         min_quantity, max_quantity, fee_amount, currency,
		         lead_time_hours, window_hours, created_at, updated_at
		  FROM shipping_templates`
	var args []interface{}
	if regionID > 0 {
		q += ` WHERE region_id = ?`
		args = append(args, regionID)
	}
	q += ` ORDER BY region_id, delivery_method, min_weight_kg`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("shipping.ListTemplates: %w", err)
	}
	defer rows.Close()

	var templates []*models.ShippingTemplate
	for rows.Next() {
		t := &models.ShippingTemplate{}
		if err := rows.Scan(&t.ID, &t.RegionID, &t.DeliveryMethod,
			&t.MinWeightKg, &t.MaxWeightKg, &t.MinQuantity, &t.MaxQuantity,
			&t.FeeAmount, &t.Currency, &t.LeadTimeHours, &t.WindowHours,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("shipping.ListTemplates: scan: %w", err)
		}
		templates = append(templates, t)
	}
	if templates == nil {
		templates = []*models.ShippingTemplate{}
	}
	return templates, rows.Err()
}

// CreateTemplate adds a new fee template.
func (s *Service) CreateTemplate(ctx context.Context, in CreateTemplateInput) (*models.ShippingTemplate, error) {
	if in.RegionID == 0 {
		return nil, errors.New("region_id is required")
	}
	if in.DeliveryMethod != "pickup" && in.DeliveryMethod != "courier" {
		return nil, errors.New("delivery_method must be 'pickup' or 'courier'")
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO shipping_templates
		   (region_id, delivery_method, min_weight_kg, max_weight_kg,
		    min_quantity, max_quantity, fee_amount, currency, lead_time_hours, window_hours)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.RegionID, in.DeliveryMethod,
		in.MinWeightKg, in.MaxWeightKg,
		in.MinQuantity, in.MaxQuantity,
		in.FeeAmount, in.Currency,
		in.LeadTimeHours, in.WindowHours,
	)
	if err != nil {
		return nil, fmt.Errorf("shipping.CreateTemplate: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.getTemplateByID(ctx, uint64(id))
}

// UpdateTemplate replaces template fields.
func (s *Service) UpdateTemplate(ctx context.Context, id uint64, in CreateTemplateInput) (*models.ShippingTemplate, error) {
	if in.DeliveryMethod != "pickup" && in.DeliveryMethod != "courier" {
		return nil, errors.New("delivery_method must be 'pickup' or 'courier'")
	}
	if in.Currency == "" {
		in.Currency = "USD"
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE shipping_templates
		 SET delivery_method=?, min_weight_kg=?, max_weight_kg=?,
		     min_quantity=?, max_quantity=?, fee_amount=?, currency=?,
		     lead_time_hours=?, window_hours=?
		 WHERE id=?`,
		in.DeliveryMethod, in.MinWeightKg, in.MaxWeightKg,
		in.MinQuantity, in.MaxQuantity, in.FeeAmount, in.Currency,
		in.LeadTimeHours, in.WindowHours, id,
	)
	if err != nil {
		return nil, fmt.Errorf("shipping.UpdateTemplate: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.getTemplateByID(ctx, id)
}

// ─── Estimate ────────────────────────────────────────────────────────────────

// EstimateInput carries the parameters for a shipping estimate calculation.
type EstimateInput struct {
	RegionID       uint64
	WeightKg       float64
	Quantity       int
	DeliveryMethod string // "pickup" | "courier"
	RequestedAt    time.Time
}

// Estimate calculates the shipping fee and estimated arrival window.
func (s *Service) Estimate(ctx context.Context, in EstimateInput) (*models.EstimateResult, error) {
	if in.DeliveryMethod == "pickup" {
		return &models.EstimateResult{Fee: 0, Currency: "USD"}, nil
	}

	// Load region for timezone and cutoff
	region, err := s.getRegionByID(ctx, in.RegionID)
	if err != nil {
		return nil, err
	}

	// Find matching template
	tmpl, err := s.findTemplate(ctx, in.RegionID, in.DeliveryMethod, in.WeightKg, in.Quantity)
	if err != nil {
		return nil, err
	}

	// Compute ETA
	etaWindow := ComputeETA(
		region.Timezone, region.CutoffTime,
		tmpl.LeadTimeHours, tmpl.WindowHours,
		in.RequestedAt,
	)

	return &models.EstimateResult{
		Fee:                    tmpl.FeeAmount,
		Currency:               tmpl.Currency,
		EstimatedArrivalWindow: etaWindow,
	}, nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (s *Service) getRegionByID(ctx context.Context, id uint64) (*models.ShippingRegion, error) {
	r := &models.ShippingRegion{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, cutoff_time, timezone, created_at, updated_at
		 FROM shipping_regions WHERE id=?`, id,
	).Scan(&r.ID, &r.Name, &r.CutoffTime, &r.Timezone, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func (s *Service) getTemplateByID(ctx context.Context, id uint64) (*models.ShippingTemplate, error) {
	t := &models.ShippingTemplate{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, region_id, delivery_method, min_weight_kg, max_weight_kg,
		        min_quantity, max_quantity, fee_amount, currency,
		        lead_time_hours, window_hours, created_at, updated_at
		 FROM shipping_templates WHERE id=?`, id,
	).Scan(&t.ID, &t.RegionID, &t.DeliveryMethod,
		&t.MinWeightKg, &t.MaxWeightKg, &t.MinQuantity, &t.MaxQuantity,
		&t.FeeAmount, &t.Currency, &t.LeadTimeHours, &t.WindowHours,
		&t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *Service) findTemplate(ctx context.Context, regionID uint64, method string, weightKg float64, quantity int) (*models.ShippingTemplate, error) {
	t := &models.ShippingTemplate{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, region_id, delivery_method, min_weight_kg, max_weight_kg,
		        min_quantity, max_quantity, fee_amount, currency,
		        lead_time_hours, window_hours, created_at, updated_at
		 FROM shipping_templates
		 WHERE region_id = ?
		   AND delivery_method = ?
		   AND min_weight_kg <= ?
		   AND max_weight_kg >= ?
		   AND min_quantity <= ?
		   AND max_quantity >= ?
		 ORDER BY fee_amount ASC
		 LIMIT 1`,
		regionID, method, weightKg, weightKg, quantity, quantity,
	).Scan(&t.ID, &t.RegionID, &t.DeliveryMethod,
		&t.MinWeightKg, &t.MaxWeightKg, &t.MinQuantity, &t.MaxQuantity,
		&t.FeeAmount, &t.Currency, &t.LeadTimeHours, &t.WindowHours,
		&t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoTemplate
	}
	return t, err
}
