package domain

import "time"

const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"

	StatusActive   = "active"
	StatusInactive = "inactive"
	StatusRevoked  = "revoked"

	AlertBlue   = "blue"
	AlertOrange = "orange"
	AlertRed    = "red"

	PlatformAndroid = "android"
	PlatformIOS     = "ios"
	PlatformWeb     = "web"
)

// User mirrors the `users` table.
type User struct {
	ID           int64     `db:"id"            json:"user_id"`
	Username     string    `db:"username"      json:"username"`
	Email        string    `db:"email"         json:"email"`
	PhoneE164    string    `db:"phone_e164"    json:"phone_e164,omitempty"`
	PasswordHash string    `db:"password_hash" json:"-"`
	Role         string    `db:"role"          json:"role"`
	Status       string    `db:"status"        json:"status"`
	CreatedAt    time.Time `db:"created_at"    json:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"    json:"updated_at,omitempty"`
}

// UserProjectAlertSubscription mirrors the `user_project_alert_subscription` table.
type UserProjectAlertSubscription struct {
	ID              int64      `db:"id"               json:"id"`
	UserID          int64      `db:"user_id"          json:"user_id"`
	ProjectID       string     `db:"project_id"       json:"project_id"`
	MinAlertLevel   string     `db:"min_alert_level"  json:"min_alert_level"`
	SMSEnabled      bool       `db:"sms_enabled"      json:"sms_enabled"`
	PushEnabled     bool       `db:"push_enabled"     json:"push_enabled"`
	EmailEnabled    bool       `db:"email_enabled"    json:"email_enabled"`
	QuietHoursStart *string    `db:"quiet_hours_start" json:"quiet_hours_start,omitempty"`
	QuietHoursEnd   *string    `db:"quiet_hours_end"   json:"quiet_hours_end,omitempty"`
	CreatedAt       time.Time  `db:"created_at"       json:"created_at"`
}

// RecipientRow is the result of the gRPC alert-recipients join query.
type RecipientRow struct {
	ID              int64   `db:"id"`
	UserID          int64   `db:"user_id"`
	Email           string  `db:"email"`
	PhoneE164       string  `db:"phone_e164"`
	MinAlertLevel   string  `db:"min_alert_level"`
	SMSEnabled      bool    `db:"sms_enabled"`
	PushEnabled     bool    `db:"push_enabled"`
	EmailEnabled    bool    `db:"email_enabled"`
	QuietHoursStart *string `db:"quiet_hours_start"`
	QuietHoursEnd   *string `db:"quiet_hours_end"`
}

// UserDevice mirrors the `user_device` table.
type UserDevice struct {
	ID          int64     `db:"id"           json:"device_id"`
	UserID      int64     `db:"user_id"      json:"user_id"`
	DeviceToken string    `db:"device_token" json:"device_token"`
	Platform    string    `db:"platform"     json:"platform"`
	AppVersion  string    `db:"app_version"  json:"app_version,omitempty"`
	LastSeenAt  time.Time `db:"last_seen_at" json:"last_seen_at"`
	Status      string    `db:"status"       json:"status"`
	CreatedAt   time.Time `db:"created_at"   json:"created_at"`
}
