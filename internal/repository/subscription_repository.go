package repository

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/tifacusil/go-user-management/internal/domain"
)

type SubscriptionRepository struct {
	db *sqlx.DB
}

func NewSubscriptionRepository(db *sqlx.DB) *SubscriptionRepository {
	return &SubscriptionRepository{db: db}
}

func (r *SubscriptionRepository) Create(ctx context.Context, s *domain.UserProjectAlertSubscription) error {
	const q = `INSERT INTO user_project_alert_subscription
	            (user_id, project_id, min_alert_level, sms_enabled, push_enabled, email_enabled,
	             quiet_hours_start, quiet_hours_end)
	            VALUES (:user_id, :project_id, :min_alert_level, :sms_enabled, :push_enabled, :email_enabled,
	            :quiet_hours_start, :quiet_hours_end)`
	res, err := r.db.NamedExecContext(ctx, q, s)
	if err != nil {
		if isDuplicate(err) {
			return fmt.Errorf("%w: subscription for this project already exists", ErrConflict)
		}
		return fmt.Errorf("insert subscription: %w", err)
	}
	id, _ := res.LastInsertId()
	s.ID = id
	return nil
}

// ListByProjectID returns subscriptions for a project joined with user contact info.
// Used by the gRPC alert-recipients server.
func (r *SubscriptionRepository) ListByProjectID(ctx context.Context, projectID string) ([]domain.RecipientRow, error) {
	const q = `
		SELECT s.user_id, u.email, u.phone_e164,
		       s.min_alert_level, s.sms_enabled, s.push_enabled, s.email_enabled,
		       s.quiet_hours_start, s.quiet_hours_end
		FROM user_project_alert_subscription s
		JOIN users u ON u.id = s.user_id
		WHERE s.project_id = ? AND u.status = 'active'`
	var rows []domain.RecipientRow
	if err := r.db.SelectContext(ctx, &rows, q, projectID); err != nil {
		return nil, fmt.Errorf("list by project: %w", err)
	}
	return rows, nil
}

func (r *SubscriptionRepository) ListByUserID(ctx context.Context, userID int64) ([]domain.UserProjectAlertSubscription, error) {
	var subs []domain.UserProjectAlertSubscription
	err := r.db.SelectContext(ctx, &subs,
		`SELECT * FROM user_project_alert_subscription WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	return subs, nil
}
