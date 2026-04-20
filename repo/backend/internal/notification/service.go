// Package notification provides in-app notification dispatch and retrieval.
//
// The dispatch pipeline:
//  1. A caller invokes Dispatch(ctx, userID, templateCode, vars).
//  2. The service loads the template, renders title+body via Go text/template.
//  3. Inserts a row into `notifications`.
//  4. If the user has notify_in_app=false, inserts a row into `notification_outbox`
//     so the message can be surfaced through an alternate channel.
package notification

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/eagle-point/service-portal/internal/models"
)

// Errors.
var (
	ErrNotFound      = errors.New("not found")
	ErrValidation    = errors.New("validation error")
	ErrTemplateParse = errors.New("template parse failed")
)

// Service provides notification dispatch + retrieval.
type Service struct {
	db *sql.DB
}

// NewService creates a Service.
func NewService(db *sql.DB) *Service { return &Service{db: db} }

// ─── Template CRUD ───────────────────────────────────────────────────────────

// ListTemplates returns all templates (admin use).
func (s *Service) ListTemplates(ctx context.Context) ([]*models.NotificationTemplate, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, code, title_template, body_template, created_at, updated_at
		 FROM notification_templates ORDER BY code ASC`)
	if err != nil {
		return nil, fmt.Errorf("notification.ListTemplates: %w", err)
	}
	defer rows.Close()

	var out []*models.NotificationTemplate
	for rows.Next() {
		t := &models.NotificationTemplate{}
		if err := rows.Scan(&t.ID, &t.Code, &t.TitleTemplate, &t.BodyTemplate,
			&t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if out == nil {
		out = []*models.NotificationTemplate{}
	}
	return out, rows.Err()
}

// GetTemplate fetches a template by code.
func (s *Service) GetTemplate(ctx context.Context, code string) (*models.NotificationTemplate, error) {
	t := &models.NotificationTemplate{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, code, title_template, body_template, created_at, updated_at
		 FROM notification_templates WHERE code = ?`, code,
	).Scan(&t.ID, &t.Code, &t.TitleTemplate, &t.BodyTemplate, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// UpsertTemplate replaces or creates a template by code.
func (s *Service) UpsertTemplate(ctx context.Context, code, title, body string) (*models.NotificationTemplate, error) {
	if strings.TrimSpace(code) == "" {
		return nil, fmt.Errorf("%w: code is required", ErrValidation)
	}
	// Verify templates parse (fail fast on admin edit)
	if _, err := template.New("title").Parse(title); err != nil {
		return nil, fmt.Errorf("%w: title: %v", ErrTemplateParse, err)
	}
	if _, err := template.New("body").Parse(body); err != nil {
		return nil, fmt.Errorf("%w: body: %v", ErrTemplateParse, err)
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notification_templates (code, title_template, body_template)
		 VALUES (?, ?, ?)
		 ON DUPLICATE KEY UPDATE title_template=VALUES(title_template),
		                         body_template=VALUES(body_template)`,
		code, title, body,
	)
	if err != nil {
		return nil, fmt.Errorf("notification.UpsertTemplate: %w", err)
	}
	return s.GetTemplate(ctx, code)
}

// ─── Dispatch ────────────────────────────────────────────────────────────────

// Render produces (title, body) for a template + variable map.
// Exported for unit tests that don't need the DB.
func Render(t *models.NotificationTemplate, vars map[string]any) (string, string, error) {
	titleTmpl, err := template.New("title").Parse(t.TitleTemplate)
	if err != nil {
		return "", "", fmt.Errorf("%w: title: %v", ErrTemplateParse, err)
	}
	bodyTmpl, err := template.New("body").Parse(t.BodyTemplate)
	if err != nil {
		return "", "", fmt.Errorf("%w: body: %v", ErrTemplateParse, err)
	}
	var titleBuf, bodyBuf bytes.Buffer
	if err := titleTmpl.Execute(&titleBuf, vars); err != nil {
		return "", "", err
	}
	if err := bodyTmpl.Execute(&bodyBuf, vars); err != nil {
		return "", "", err
	}
	return titleBuf.String(), bodyBuf.String(), nil
}

// Dispatch creates a notification for userID using the named template + variables.
// If the user's notify_in_app preference is false, an outbox entry is also created.
func (s *Service) Dispatch(ctx context.Context, userID uint64, code string, vars map[string]any) (*models.Notification, error) {
	if userID == 0 {
		return nil, fmt.Errorf("%w: user_id required", ErrValidation)
	}
	tmpl, err := s.GetTemplate(ctx, code)
	if err != nil {
		return nil, err
	}
	title, body, err := Render(tmpl, vars)
	if err != nil {
		return nil, err
	}

	res, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications (user_id, template_code, title, body)
		 VALUES (?, ?, ?, ?)`,
		userID, code, title, body,
	)
	if err != nil {
		return nil, fmt.Errorf("notification.Dispatch: %w", err)
	}
	id, _ := res.LastInsertId()

	// Check preference: if notify_in_app = false, add to outbox
	notifyInApp, _ := s.lookupNotifyInApp(ctx, userID)
	if !notifyInApp {
		_, _ = s.db.ExecContext(ctx,
			`INSERT INTO notification_outbox (user_id, notification_id, status) VALUES (?, ?, 'pending')`,
			userID, id,
		)
	}

	return s.Get(ctx, uint64(id))
}

// lookupNotifyInApp returns whether the user has enabled in-app notifications.
// Missing row defaults to true (opt-out model).
func (s *Service) lookupNotifyInApp(ctx context.Context, userID uint64) (bool, error) {
	var enabled bool
	err := s.db.QueryRowContext(ctx,
		`SELECT notify_in_app FROM user_preferences WHERE user_id = ?`, userID,
	).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return true, err
	}
	return enabled, nil
}

// ─── Retrieval ───────────────────────────────────────────────────────────────

// Get returns a single notification.
func (s *Service) Get(ctx context.Context, id uint64) (*models.Notification, error) {
	n := &models.Notification{}
	var code sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, template_code, title, body, is_read, created_at
		 FROM notifications WHERE id = ?`, id,
	).Scan(&n.ID, &n.UserID, &code, &n.Title, &n.Body, &n.IsRead, &n.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if code.Valid {
		n.TemplateCode = code.String
	}
	return n, nil
}

// NotificationPage is a cursor-paginated notification list.
type NotificationPage struct {
	Items      []*models.Notification `json:"items"`
	NextCursor uint64                 `json:"next_cursor"`
}

// List returns the user's notifications, optionally filtered by read status.
//   - readState: "" = any, "read" = only read, "unread" = only unread
func (s *Service) List(ctx context.Context, userID uint64, readState string, cursor uint64, limit int) (*NotificationPage, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	q := `SELECT id, user_id, template_code, title, body, is_read, created_at
	      FROM notifications WHERE user_id = ?`
	args := []interface{}{userID}
	if readState == "read" {
		q += ` AND is_read = 1`
	} else if readState == "unread" {
		q += ` AND is_read = 0`
	}
	if cursor > 0 {
		q += ` AND id < ?`
		args = append(args, cursor)
	}
	q += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("notification.List: %w", err)
	}
	defer rows.Close()

	var all []*models.Notification
	for rows.Next() {
		n := &models.Notification{}
		var code sql.NullString
		if err := rows.Scan(&n.ID, &n.UserID, &code, &n.Title, &n.Body, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, err
		}
		if code.Valid {
			n.TemplateCode = code.String
		}
		all = append(all, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	page := &NotificationPage{Items: all}
	if len(all) > limit {
		page.Items = all[:limit]
		page.NextCursor = all[limit-1].ID
	}
	if page.Items == nil {
		page.Items = []*models.Notification{}
	}
	return page, nil
}

// UnreadCount returns the user's unread notification count.
func (s *Service) UnreadCount(ctx context.Context, userID uint64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND is_read = 0`,
		userID,
	).Scan(&n)
	return n, err
}

// MarkRead flips is_read=1 for a single notification owned by userID.
func (s *Service) MarkRead(ctx context.Context, id, userID uint64) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET is_read = 1 WHERE id = ? AND user_id = ?`,
		id, userID,
	)
	if err != nil {
		return fmt.Errorf("notification.MarkRead: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkAllRead flips is_read=1 for every notification belonging to the user.
func (s *Service) MarkAllRead(ctx context.Context, userID uint64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET is_read = 1 WHERE user_id = ? AND is_read = 0`,
		userID,
	)
	if err != nil {
		return 0, fmt.Errorf("notification.MarkAllRead: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ─── Outbox ──────────────────────────────────────────────────────────────────

// ListOutbox returns outbox entries (newest first) for a user.
// Joins with notifications so the caller sees the full message.
func (s *Service) ListOutbox(ctx context.Context, userID uint64, limit int) ([]*models.NotificationOutboxEntry, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT o.id, o.user_id, o.notification_id, o.status, o.attempts, o.last_attempt_at, o.created_at,
		        n.id, n.user_id, n.template_code, n.title, n.body, n.is_read, n.created_at
		 FROM notification_outbox o
		 JOIN notifications n ON o.notification_id = n.id
		 WHERE o.user_id = ?
		 ORDER BY o.id DESC
		 LIMIT ?`,
		userID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("notification.ListOutbox: %w", err)
	}
	defer rows.Close()

	var out []*models.NotificationOutboxEntry
	for rows.Next() {
		e := &models.NotificationOutboxEntry{Notification: &models.Notification{}}
		var lastAttempt sql.NullTime
		var code sql.NullString
		if err := rows.Scan(&e.ID, &e.UserID, &e.NotificationID, &e.Status, &e.Attempts, &lastAttempt, &e.CreatedAt,
			&e.Notification.ID, &e.Notification.UserID, &code, &e.Notification.Title, &e.Notification.Body,
			&e.Notification.IsRead, &e.Notification.CreatedAt); err != nil {
			return nil, err
		}
		if lastAttempt.Valid {
			t := lastAttempt.Time
			e.LastAttemptAt = &t
		}
		if code.Valid {
			e.Notification.TemplateCode = code.String
		}
		out = append(out, e)
	}
	if out == nil {
		out = []*models.NotificationOutboxEntry{}
	}
	return out, rows.Err()
}

// SeedDefaultTemplates inserts the default set of templates if they do not already exist.
// Called once at router construction so tests and fresh deployments have a working set.
func SeedDefaultTemplates(ctx context.Context, db *sql.DB) error {
	defaults := []struct {
		code, title, body string
	}{
		{
			models.NotifTicketStatusChange,
			"Ticket #{{.TicketID}} is now {{.Status}}",
			"Your service request has moved to status: {{.Status}}.",
		},
		{
			models.NotifSLABreach,
			"Ticket #{{.TicketID}} SLA breached",
			"The SLA deadline for ticket #{{.TicketID}} has passed.",
		},
		{
			models.NotifAccountLockout,
			"Your account has been locked",
			"Due to repeated failed login attempts, your account is temporarily locked until {{.Until}}.",
		},
		{
			models.NotifPostingFreeze,
			"Posting temporarily disabled",
			"Due to content-policy violations, posting is disabled until {{.Until}}.",
		},
		{
			models.NotifApprovalReminder,
			"Moderation queue has {{.Count}} pending items",
			"There are {{.Count}} items waiting for moderator review.",
		},
		{
			models.NotifUpcomingStart,
			"Reminder: ticket #{{.TicketID}} starts soon",
			"Your scheduled service for ticket #{{.TicketID}} begins at {{.Start}}.",
		},
		{
			models.NotifUpcomingEnd,
			"Reminder: ticket #{{.TicketID}} ends soon",
			"Your scheduled service for ticket #{{.TicketID}} is expected to end at {{.End}}.",
		},
	}

	for _, d := range defaults {
		_, err := db.ExecContext(ctx,
			`INSERT IGNORE INTO notification_templates (code, title_template, body_template)
			 VALUES (?, ?, ?)`,
			d.code, d.title, d.body,
		)
		if err != nil {
			return fmt.Errorf("seed template %s: %w", d.code, err)
		}
	}
	return nil
}
