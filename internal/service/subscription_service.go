package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/tifacusil/go-user-management/internal/client"
	"github.com/tifacusil/go-user-management/internal/domain"
	"github.com/tifacusil/go-user-management/internal/repository"
)

var ErrProjectNotFound = errors.New("project not found")
var ErrServiceUnavailable = errors.New("upstream service unavailable")

type SubscriptionService struct {
	subs    *repository.SubscriptionRepository
	project *client.ProjectClient
}

func NewSubscriptionService(subs *repository.SubscriptionRepository, project *client.ProjectClient) *SubscriptionService {
	return &SubscriptionService{subs: subs, project: project}
}

type CreateSubscriptionInput struct {
	ProjectID       string  `json:"project_id"       validate:"required"`
	MinAlertLevel   string  `json:"min_alert_level"  validate:"required,oneof=blue orange red"`
	SMSEnabled      bool    `json:"sms_enabled"`
	PushEnabled     bool    `json:"push_enabled"`
	EmailEnabled    bool    `json:"email_enabled"`
	QuietHoursStart *string `json:"quiet_hours_start" validate:"omitempty"`
	QuietHoursEnd   *string `json:"quiet_hours_end"   validate:"omitempty"`
}

func (s *SubscriptionService) Create(ctx context.Context, userID int64, in CreateSubscriptionInput) (*domain.UserProjectAlertSubscription, error) {
	if err := s.project.ProjectExists(ctx, in.ProjectID); err != nil {
		if errors.Is(err, client.ErrProjectNotFound) {
			return nil, ErrProjectNotFound
		}
		if errors.Is(err, client.ErrCircuitOpen) {
			return nil, ErrServiceUnavailable
		}
		return nil, fmt.Errorf("verify project: %w", err)
	}

	sub := &domain.UserProjectAlertSubscription{
		UserID:          userID,
		ProjectID:       in.ProjectID,
		MinAlertLevel:   in.MinAlertLevel,
		SMSEnabled:      in.SMSEnabled,
		PushEnabled:     in.PushEnabled,
		EmailEnabled:    in.EmailEnabled,
		QuietHoursStart: in.QuietHoursStart,
		QuietHoursEnd:   in.QuietHoursEnd,
	}

	if err := s.subs.Create(ctx, sub); err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *SubscriptionService) ListByUser(ctx context.Context, userID int64) ([]domain.UserProjectAlertSubscription, error) {
	return s.subs.ListByUserID(ctx, userID)
}
