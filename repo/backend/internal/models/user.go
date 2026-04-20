package models

import "time"

// User represents a portal user.
type User struct {
	ID                 uint64
	Username           string
	Email              string
	PasswordHash       string
	DisplayName        string
	PhoneEncrypted     []byte
	AvatarURL          string
	Bio                string
	IsActive           bool
	IsDeleted          bool
	AnonymizedAt       *time.Time
	PostingFreezeUntil *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
	Roles              []string // populated on load; not stored in users table
}

// HasRole returns true if the user holds the given role.
func (u *User) HasRole(role string) bool {
	for _, r := range u.Roles {
		if r == role {
			return true
		}
	}
	return false
}

// IsPostingFrozen returns true if the user is currently under a posting freeze.
func (u *User) IsPostingFrozen() bool {
	if u.PostingFreezeUntil == nil {
		return false
	}
	return time.Now().Before(*u.PostingFreezeUntil)
}

// SafeView returns a map safe to send in API responses (no password hash, no encrypted bytes).
func (u *User) SafeView() map[string]interface{} {
	return map[string]interface{}{
		"id":           u.ID,
		"username":     u.Username,
		"display_name": u.DisplayName,
		"avatar_url":   u.AvatarURL,
		"bio":          u.Bio,
		"roles":        u.Roles,
		"created_at":   u.CreatedAt,
	}
}

// Role constants
const (
	RoleRegularUser  = "regular_user"
	RoleServiceAgent = "service_agent"
	RoleModerator    = "moderator"
	RoleAdministrator = "administrator"
	RoleDataOperator = "data_operator"
)
