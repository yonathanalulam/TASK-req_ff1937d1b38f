package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/eagle-point/service-portal/internal/models"
)

// ErrNotFound is returned when the requested entity does not exist.
var ErrNotFound = errors.New("not found")

// ErrForbidden is returned when the caller lacks permission to modify an offering.
var ErrForbidden = errors.New("forbidden")

// Service provides service-category and service-offering business logic.
type Service struct {
	db *sql.DB
}

// NewService creates a catalog Service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// ─── Categories ──────────────────────────────────────────────────────────────

// CreateCategoryInput carries fields for a new service category.
type CreateCategoryInput struct {
	Name                  string
	Slug                  string
	Description           string
	ResponseTimeMinutes   int
	CompletionTimeMinutes int
}

// ListCategories returns all service categories.
func (s *Service) ListCategories(ctx context.Context) ([]*models.ServiceCategory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, slug, description, response_time_minutes, completion_time_minutes,
		        created_at, updated_at
		 FROM service_categories ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("catalog.ListCategories: %w", err)
	}
	defer rows.Close()

	var cats []*models.ServiceCategory
	for rows.Next() {
		c := &models.ServiceCategory{}
		if err := rows.Scan(&c.ID, &c.Name, &c.Slug, &c.Description,
			&c.ResponseTimeMinutes, &c.CompletionTimeMinutes,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("catalog.ListCategories: scan: %w", err)
		}
		cats = append(cats, c)
	}
	if cats == nil {
		cats = []*models.ServiceCategory{}
	}
	return cats, rows.Err()
}

// CreateCategory adds a new service category (Administrator only, enforced at handler level).
func (s *Service) CreateCategory(ctx context.Context, in CreateCategoryInput) (*models.ServiceCategory, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Slug = strings.TrimSpace(strings.ToLower(in.Slug))
	if in.Name == "" {
		return nil, errors.New("name is required")
	}
	if in.Slug == "" {
		return nil, errors.New("slug is required")
	}
	if in.ResponseTimeMinutes <= 0 {
		in.ResponseTimeMinutes = 60
	}
	if in.CompletionTimeMinutes <= 0 {
		in.CompletionTimeMinutes = 480
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO service_categories
		   (name, slug, description, response_time_minutes, completion_time_minutes)
		 VALUES (?, ?, ?, ?, ?)`,
		in.Name, in.Slug, in.Description,
		in.ResponseTimeMinutes, in.CompletionTimeMinutes,
	)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate entry") {
			return nil, errors.New("category name or slug already exists")
		}
		return nil, fmt.Errorf("catalog.CreateCategory: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.getCategoryByID(ctx, uint64(id))
}

// UpdateCategory replaces mutable category fields.
func (s *Service) UpdateCategory(ctx context.Context, id uint64, in CreateCategoryInput) (*models.ServiceCategory, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return nil, errors.New("name is required")
	}
	if in.ResponseTimeMinutes <= 0 {
		in.ResponseTimeMinutes = 60
	}
	if in.CompletionTimeMinutes <= 0 {
		in.CompletionTimeMinutes = 480
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE service_categories
		 SET name=?, description=?, response_time_minutes=?, completion_time_minutes=?
		 WHERE id=?`,
		in.Name, in.Description, in.ResponseTimeMinutes, in.CompletionTimeMinutes, id,
	)
	if err != nil {
		return nil, fmt.Errorf("catalog.UpdateCategory: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	return s.getCategoryByID(ctx, id)
}

// DeleteCategory removes a category by ID.
func (s *Service) DeleteCategory(ctx context.Context, id uint64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM service_categories WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("catalog.DeleteCategory: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Offerings ───────────────────────────────────────────────────────────────

// OfferingFilter carries optional filters for listing offerings.
type OfferingFilter struct {
	CategoryID uint64 // 0 = all
	Active     int    // -1 = all, 1 = active only, 0 = inactive only
}

// OfferingPage is a paginated list of offerings.
type OfferingPage struct {
	Items      []*models.ServiceOffering `json:"items"`
	NextCursor uint64                    `json:"next_cursor"`
}

// CreateOfferingInput carries fields for a new service offering.
type CreateOfferingInput struct {
	CategoryID      uint64
	Name            string
	Description     string
	BasePrice       float64
	DurationMinutes int
}

// ListOfferings returns a cursor-paginated, filtered list of offerings.
func (s *Service) ListOfferings(ctx context.Context, f OfferingFilter, cursor uint64, limit int) (*OfferingPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, category_id, name, description, base_price,
		        duration_minutes, active_status, created_at, updated_at
		 FROM service_offerings
		 WHERE (? = 0 OR category_id = ?)
		   AND (? = -1 OR active_status = ?)
		   AND (? = 0 OR id < ?)
		 ORDER BY id DESC
		 LIMIT ?`,
		f.CategoryID, f.CategoryID,
		f.Active, f.Active,
		cursor, cursor,
		limit+1,
	)
	if err != nil {
		return nil, fmt.Errorf("catalog.ListOfferings: %w", err)
	}
	defer rows.Close()

	var items []*models.ServiceOffering
	for rows.Next() {
		o, err := scanOffering(rows)
		if err != nil {
			return nil, fmt.Errorf("catalog.ListOfferings: scan: %w", err)
		}
		items = append(items, o)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("catalog.ListOfferings: rows: %w", err)
	}

	page := &OfferingPage{Items: items}
	if len(items) > limit {
		page.Items = items[:limit]
		page.NextCursor = items[limit-1].ID
	}
	if page.Items == nil {
		page.Items = []*models.ServiceOffering{}
	}
	return page, nil
}

// GetOffering returns a single offering by ID.
func (s *Service) GetOffering(ctx context.Context, id uint64) (*models.ServiceOffering, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, agent_id, category_id, name, description, base_price,
		        duration_minutes, active_status, created_at, updated_at
		 FROM service_offerings WHERE id = ?`, id)
	o, err := scanOffering(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

// CreateOffering adds a new offering owned by agentID.
func (s *Service) CreateOffering(ctx context.Context, agentID uint64, in CreateOfferingInput) (*models.ServiceOffering, error) {
	if err := validateOffering(in); err != nil {
		return nil, err
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO service_offerings
		   (agent_id, category_id, name, description, base_price, duration_minutes, active_status)
		 VALUES (?, ?, ?, ?, ?, ?, 1)`,
		agentID, in.CategoryID, strings.TrimSpace(in.Name),
		in.Description, in.BasePrice, in.DurationMinutes,
	)
	if err != nil {
		return nil, fmt.Errorf("catalog.CreateOffering: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetOffering(ctx, uint64(id))
}

// UpdateOffering replaces offering fields, enforcing ownership.
func (s *Service) UpdateOffering(ctx context.Context, id, callerID uint64, callerRoles []string, in CreateOfferingInput) (*models.ServiceOffering, error) {
	if err := validateOffering(in); err != nil {
		return nil, err
	}

	o, err := s.GetOffering(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canModify(callerID, callerRoles, o.AgentID) {
		return nil, ErrForbidden
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE service_offerings
		 SET category_id=?, name=?, description=?, base_price=?, duration_minutes=?
		 WHERE id=?`,
		in.CategoryID, strings.TrimSpace(in.Name), in.Description,
		in.BasePrice, in.DurationMinutes, id,
	)
	if err != nil {
		return nil, fmt.Errorf("catalog.UpdateOffering: %w", err)
	}
	return s.GetOffering(ctx, id)
}

// ToggleStatus enables or disables an offering, enforcing ownership.
func (s *Service) ToggleStatus(ctx context.Context, id, callerID uint64, callerRoles []string, active bool) (*models.ServiceOffering, error) {
	o, err := s.GetOffering(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canModify(callerID, callerRoles, o.AgentID) {
		return nil, ErrForbidden
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE service_offerings SET active_status=? WHERE id=?`, active, id)
	if err != nil {
		return nil, fmt.Errorf("catalog.ToggleStatus: %w", err)
	}
	return s.GetOffering(ctx, id)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (s *Service) getCategoryByID(ctx context.Context, id uint64) (*models.ServiceCategory, error) {
	c := &models.ServiceCategory{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, response_time_minutes, completion_time_minutes,
		        created_at, updated_at
		 FROM service_categories WHERE id=?`, id,
	).Scan(&c.ID, &c.Name, &c.Slug, &c.Description,
		&c.ResponseTimeMinutes, &c.CompletionTimeMinutes,
		&c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOffering(rs rowScanner) (*models.ServiceOffering, error) {
	o := &models.ServiceOffering{}
	err := rs.Scan(&o.ID, &o.AgentID, &o.CategoryID, &o.Name, &o.Description,
		&o.BasePrice, &o.DurationMinutes, &o.ActiveStatus,
		&o.CreatedAt, &o.UpdatedAt)
	return o, err
}

func validateOffering(in CreateOfferingInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("name is required")
	}
	if in.CategoryID == 0 {
		return errors.New("category_id is required")
	}
	if in.BasePrice < 0 {
		return errors.New("base_price must be non-negative")
	}
	if in.DurationMinutes <= 0 {
		return errors.New("duration_minutes must be positive")
	}
	return nil
}

// canModify returns true if the caller owns the offering or is an administrator.
func canModify(callerID uint64, callerRoles []string, ownerID uint64) bool {
	if callerID == ownerID {
		return true
	}
	for _, r := range callerRoles {
		if r == "administrator" {
			return true
		}
	}
	return false
}

// CanModify is the exported version for testing.
func CanModify(callerID uint64, callerRoles []string, ownerID uint64) bool {
	return canModify(callerID, callerRoles, ownerID)
}
