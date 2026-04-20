package ticket

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/eagle-point/service-portal/internal/models"
)

// Errors.
var (
	ErrNotFound          = errors.New("not found")
	ErrForbidden         = errors.New("forbidden")
	ErrInvalidTransition = errors.New("invalid status transition")
	ErrValidation        = errors.New("validation error")
)

// DispatchFunc is a pluggable notification hook. It is invoked fire-and-forget
// on ticket lifecycle events; returning an error is logged but does not fail
// the operation. Kept as a function type so the ticket package does not depend
// on the notification package.
type DispatchFunc func(ctx context.Context, userID uint64, code string, vars map[string]any) error

// Service provides ticket business logic.
type Service struct {
	db         *sql.DB
	storageDir string // base directory for attachment storage (e.g., "storage/uploads")
	notify     DispatchFunc
}

// NewService creates a ticket Service.
func NewService(db *sql.DB, storageDir string) *Service {
	if storageDir == "" {
		storageDir = "storage/uploads"
	}
	return &Service{db: db, storageDir: storageDir}
}

// StorageDir returns the base path for uploads.
func (s *Service) StorageDir() string { return s.storageDir }

// SetNotifier wires an optional dispatch hook. nil disables notifications.
func (s *Service) SetNotifier(fn DispatchFunc) { s.notify = fn }

// ─── Input types ─────────────────────────────────────────────────────────────

// CreateInput carries fields for a new ticket.
type CreateInput struct {
	UserID         uint64
	OfferingID     uint64
	CategoryID     uint64
	AddressID      uint64
	PreferredStart time.Time
	PreferredEnd   time.Time
	DeliveryMethod string
	ShippingFee    float64
}

// ListFilter carries filters for ticket list queries.
type ListFilter struct {
	Status string // "" = any
	UserID uint64 // 0 = any
}

// ─── Ticket CRUD ─────────────────────────────────────────────────────────────

// Create inserts a new ticket in Accepted state and computes the SLA deadline.
func (s *Service) Create(ctx context.Context, in CreateInput) (*models.Ticket, error) {
	if in.UserID == 0 || in.OfferingID == 0 || in.CategoryID == 0 || in.AddressID == 0 {
		return nil, fmt.Errorf("%w: required ids missing", ErrValidation)
	}
	if in.PreferredEnd.Before(in.PreferredStart) {
		return nil, fmt.Errorf("%w: preferred_end must be after preferred_start", ErrValidation)
	}
	if in.DeliveryMethod == "" {
		in.DeliveryMethod = models.DeliveryPickup
	}
	if in.DeliveryMethod != models.DeliveryPickup && in.DeliveryMethod != models.DeliveryCourier {
		return nil, fmt.Errorf("%w: delivery_method must be pickup or courier", ErrValidation)
	}

	// Object-level authorization: the caller must own the referenced address.
	// Without this check an authenticated user could bind a ticket to another
	// user's address by id-guessing (IDOR).
	var addrOwner uint64
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id FROM addresses WHERE id = ?`, in.AddressID,
	).Scan(&addrOwner)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: address not found", ErrValidation)
		}
		return nil, fmt.Errorf("ticket.Create: load address: %w", err)
	}
	if addrOwner != in.UserID {
		return nil, ErrForbidden
	}

	// Compute SLA deadline from category response time
	var responseMinutes int
	err = s.db.QueryRowContext(ctx,
		`SELECT response_time_minutes FROM service_categories WHERE id = ?`, in.CategoryID,
	).Scan(&responseMinutes)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: category not found", ErrValidation)
		}
		return nil, fmt.Errorf("ticket.Create: load category: %w", err)
	}
	slaDeadline := time.Now().UTC().Add(time.Duration(responseMinutes) * time.Minute)

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO tickets
		   (user_id, offering_id, category_id, address_id, preferred_start, preferred_end,
		    delivery_method, shipping_fee, status, sla_deadline)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'Accepted', ?)`,
		in.UserID, in.OfferingID, in.CategoryID, in.AddressID,
		in.PreferredStart, in.PreferredEnd,
		in.DeliveryMethod, in.ShippingFee, slaDeadline,
	)
	if err != nil {
		return nil, fmt.Errorf("ticket.Create: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.Get(ctx, uint64(id))
}

// Get returns a ticket by ID.
func (s *Service) Get(ctx context.Context, id uint64) (*models.Ticket, error) {
	return s.scanTicket(s.db.QueryRowContext(ctx, selectTicketSQL+` WHERE id = ?`, id))
}

// List returns tickets filtered by the caller's role/scope.
//   - Regular users see their own tickets
//   - Service agents see tickets assigned to them OR unassigned
//   - Administrators see all tickets
func (s *Service) List(ctx context.Context, callerID uint64, callerRoles []string, filter ListFilter) ([]*models.Ticket, error) {
	q := selectTicketSQL
	var conds []string
	var args []interface{}

	if hasRole(callerRoles, models.RoleAdministrator) {
		// No scope restriction
	} else if hasRole(callerRoles, models.RoleServiceAgent) {
		conds = append(conds, `(assigned_agent_id = ? OR assigned_agent_id IS NULL)`)
		args = append(args, callerID)
	} else {
		conds = append(conds, `user_id = ?`)
		args = append(args, callerID)
	}

	if filter.Status != "" {
		conds = append(conds, `status = ?`)
		args = append(args, filter.Status)
	}

	if len(conds) > 0 {
		q += ` WHERE ` + strings.Join(conds, ` AND `)
	}
	q += ` ORDER BY id DESC LIMIT 100`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ticket.List: %w", err)
	}
	defer rows.Close()

	var tickets []*models.Ticket
	for rows.Next() {
		t, err := s.scanTicket(rows)
		if err != nil {
			return nil, fmt.Errorf("ticket.List: scan: %w", err)
		}
		tickets = append(tickets, t)
	}
	if tickets == nil {
		tickets = []*models.Ticket{}
	}
	return tickets, rows.Err()
}

// CanView returns true if the caller is allowed to see this ticket.
func CanView(callerID uint64, callerRoles []string, t *models.Ticket) bool {
	if hasRole(callerRoles, models.RoleAdministrator) {
		return true
	}
	if hasRole(callerRoles, models.RoleServiceAgent) {
		return true // agents can view all (scoping is for list); per-agent assignment reserved
	}
	return t.UserID == callerID
}

// ─── Status transitions ──────────────────────────────────────────────────────

// UpdateStatus transitions a ticket to a new status, enforcing role and transition rules.
func (s *Service) UpdateStatus(ctx context.Context, ticketID, callerID uint64, callerRoles []string, newStatus, cancelReason string) (*models.Ticket, error) {
	t, err := s.Get(ctx, ticketID)
	if err != nil {
		return nil, err
	}

	if err := CheckTransition(t.Status, newStatus, callerID, callerRoles, t.UserID); err != nil {
		return nil, err
	}

	// Auto-assign agent on first Dispatched transition
	var setAgent bool
	if newStatus == models.TicketStatusDispatched && t.AssignedAgentID == nil && hasRole(callerRoles, models.RoleServiceAgent) {
		setAgent = true
	}

	var execErr error
	if newStatus == models.TicketStatusCancelled && cancelReason != "" {
		_, execErr = s.db.ExecContext(ctx,
			`UPDATE tickets SET status = ?, cancel_reason = ? WHERE id = ?`,
			newStatus, cancelReason, ticketID)
	} else if setAgent {
		_, execErr = s.db.ExecContext(ctx,
			`UPDATE tickets SET status = ?, assigned_agent_id = ? WHERE id = ?`,
			newStatus, callerID, ticketID)
	} else {
		_, execErr = s.db.ExecContext(ctx,
			`UPDATE tickets SET status = ? WHERE id = ?`, newStatus, ticketID)
	}
	if execErr != nil {
		return nil, fmt.Errorf("ticket.UpdateStatus: %w", execErr)
	}
	updated, err := s.Get(ctx, ticketID)
	if err == nil && s.notify != nil {
		_ = s.notify(ctx, updated.UserID, models.NotifTicketStatusChange, map[string]any{
			"TicketID": updated.ID,
			"Status":   updated.Status,
		})
		// When service begins, the end time becomes the next upcoming milestone
		// for the customer — fire an upcoming_end reminder so the notification
		// center shows both start and end semantics (requirement parity with
		// upcoming_start).
		if updated.Status == models.TicketStatusInService {
			_ = s.notify(ctx, updated.UserID, models.NotifUpcomingEnd, map[string]any{
				"TicketID": updated.ID,
				"End":      updated.PreferredEnd.Format(time.RFC3339),
			})
		}
	}
	return updated, err
}

// CheckTransition validates a proposed status change against the role matrix.
// Exported for unit testing.
func CheckTransition(current, next string, callerID uint64, callerRoles []string, ownerID uint64) error {
	if current == next {
		return fmt.Errorf("%w: already in that status", ErrInvalidTransition)
	}

	// Administrator can transition any non-final ticket to Closed, or force any transition.
	if hasRole(callerRoles, models.RoleAdministrator) {
		if current == models.TicketStatusClosed || current == models.TicketStatusCancelled {
			return fmt.Errorf("%w: ticket is final", ErrInvalidTransition)
		}
		return nil
	}

	// Service agent: forward through lifecycle on tickets they can work.
	if hasRole(callerRoles, models.RoleServiceAgent) {
		valid := map[string]string{
			models.TicketStatusAccepted:   models.TicketStatusDispatched,
			models.TicketStatusDispatched: models.TicketStatusInService,
			models.TicketStatusInService:  models.TicketStatusCompleted,
		}
		if valid[current] == next {
			return nil
		}
		return fmt.Errorf("%w: agent cannot move %s → %s", ErrInvalidTransition, current, next)
	}

	// Regular user: cancel before dispatch only, on own ticket.
	if callerID != ownerID {
		return ErrForbidden
	}
	if next != models.TicketStatusCancelled {
		return fmt.Errorf("%w: users may only cancel", ErrInvalidTransition)
	}
	if current != models.TicketStatusAccepted {
		return fmt.Errorf("%w: cannot cancel after dispatch", ErrInvalidTransition)
	}
	return nil
}

// ─── Notes ───────────────────────────────────────────────────────────────────

// CreateNote appends a note to a ticket.
func (s *Service) CreateNote(ctx context.Context, ticketID, authorID uint64, content string) (*models.TicketNote, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("%w: content required", ErrValidation)
	}
	// Verify ticket exists
	if _, err := s.Get(ctx, ticketID); err != nil {
		return nil, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO ticket_notes (ticket_id, author_id, content) VALUES (?, ?, ?)`,
		ticketID, authorID, content)
	if err != nil {
		return nil, fmt.Errorf("ticket.CreateNote: %w", err)
	}
	id, _ := res.LastInsertId()
	n := &models.TicketNote{}
	err = s.db.QueryRowContext(ctx,
		`SELECT id, ticket_id, author_id, content, created_at FROM ticket_notes WHERE id = ?`, id,
	).Scan(&n.ID, &n.TicketID, &n.AuthorID, &n.Content, &n.CreatedAt)
	return n, err
}

// ListNotes returns all notes for a ticket in chronological order.
func (s *Service) ListNotes(ctx context.Context, ticketID uint64) ([]*models.TicketNote, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ticket_id, author_id, content, created_at
		 FROM ticket_notes WHERE ticket_id = ? ORDER BY id ASC`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("ticket.ListNotes: %w", err)
	}
	defer rows.Close()

	var out []*models.TicketNote
	for rows.Next() {
		n := &models.TicketNote{}
		if err := rows.Scan(&n.ID, &n.TicketID, &n.AuthorID, &n.Content, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	if out == nil {
		out = []*models.TicketNote{}
	}
	return out, rows.Err()
}

// ─── Attachments ─────────────────────────────────────────────────────────────

// CountAttachments returns the number of attachments currently on a ticket.
func (s *Service) CountAttachments(ctx context.Context, ticketID uint64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ticket_attachments WHERE ticket_id = ?`, ticketID,
	).Scan(&n)
	return n, err
}

// RecordAttachment inserts an attachment row after the file has been saved to disk.
func (s *Service) RecordAttachment(ctx context.Context, ticketID uint64, filename, originalName, mimeType string, size uint64, storagePath string) (*models.TicketAttachment, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO ticket_attachments (ticket_id, filename, original_name, mime_type, size_bytes, storage_path)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ticketID, filename, originalName, mimeType, size, storagePath,
	)
	if err != nil {
		return nil, fmt.Errorf("ticket.RecordAttachment: %w", err)
	}
	id, _ := res.LastInsertId()
	return s.GetAttachment(ctx, uint64(id))
}

// GetAttachment returns a single attachment row.
func (s *Service) GetAttachment(ctx context.Context, id uint64) (*models.TicketAttachment, error) {
	a := &models.TicketAttachment{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, ticket_id, filename, original_name, mime_type, size_bytes, storage_path, created_at
		 FROM ticket_attachments WHERE id = ?`, id,
	).Scan(&a.ID, &a.TicketID, &a.Filename, &a.OriginalName, &a.MimeType, &a.SizeBytes, &a.StoragePath, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// ListAttachments returns all attachments for a ticket.
func (s *Service) ListAttachments(ctx context.Context, ticketID uint64) ([]*models.TicketAttachment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, ticket_id, filename, original_name, mime_type, size_bytes, storage_path, created_at
		 FROM ticket_attachments WHERE ticket_id = ? ORDER BY id ASC`, ticketID)
	if err != nil {
		return nil, fmt.Errorf("ticket.ListAttachments: %w", err)
	}
	defer rows.Close()

	var out []*models.TicketAttachment
	for rows.Next() {
		a := &models.TicketAttachment{}
		if err := rows.Scan(&a.ID, &a.TicketID, &a.Filename, &a.OriginalName,
			&a.MimeType, &a.SizeBytes, &a.StoragePath, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if out == nil {
		out = []*models.TicketAttachment{}
	}
	return out, rows.Err()
}

// DeleteAttachment removes an attachment row. The caller is responsible for deleting the file on disk.
func (s *Service) DeleteAttachment(ctx context.Context, id uint64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM ticket_attachments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("ticket.DeleteAttachment: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

const selectTicketSQL = `SELECT id, user_id, assigned_agent_id, offering_id, category_id, address_id,
	preferred_start, preferred_end, delivery_method, shipping_fee, status,
	sla_deadline, sla_breached, cancel_reason, created_at, updated_at
	FROM tickets`

type rowScanner interface {
	Scan(dest ...any) error
}

func (s *Service) scanTicket(rs rowScanner) (*models.Ticket, error) {
	t := &models.Ticket{}
	var assignedAgentID sql.NullInt64
	var slaDeadline sql.NullTime
	var cancelReason sql.NullString

	err := rs.Scan(&t.ID, &t.UserID, &assignedAgentID, &t.OfferingID, &t.CategoryID, &t.AddressID,
		&t.PreferredStart, &t.PreferredEnd, &t.DeliveryMethod, &t.ShippingFee, &t.Status,
		&slaDeadline, &t.SLABreached, &cancelReason, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if assignedAgentID.Valid {
		id := uint64(assignedAgentID.Int64)
		t.AssignedAgentID = &id
	}
	if slaDeadline.Valid {
		d := slaDeadline.Time
		t.SLADeadline = &d
	}
	if cancelReason.Valid {
		t.CancelReason = cancelReason.String
	}
	return t, nil
}

func hasRole(roles []string, target string) bool {
	for _, r := range roles {
		if r == target {
			return true
		}
	}
	return false
}
