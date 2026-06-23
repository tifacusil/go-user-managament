package grpc

import (
	"context"
	"strconv"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/tifacusil/go-user-management/internal/domain"
	subpb "github.com/tifacusil/go-user-management/internal/grpc/usermanagementpb"
)

// subscriptionLister + deviceLister are the repository slices SubscriptionServer
// needs. Narrow interfaces keep the server testable with fakes; the concrete
// *repository.SubscriptionRepository / *repository.DeviceRepository satisfy them.
type subscriptionLister interface {
	ListByProjectID(ctx context.Context, projectID string) ([]domain.RecipientRow, error)
}

type deviceLister interface {
	ListActiveByUserID(ctx context.Context, userID int64) ([]domain.UserDevice, error)
}

// SubscriptionServer implements usermanagement.v1.SubscriptionService, the
// contract owned by the early-warning consumer. It returns raw subscriptions
// per project; the consumer applies push_enabled / min_alert_level / quiet-hour
// filtering itself.
type SubscriptionServer struct {
	subpb.UnimplementedSubscriptionServiceServer
	subs    subscriptionLister
	devices deviceLister
}

func NewSubscriptionServer(subs subscriptionLister, devices deviceLister) *SubscriptionServer {
	return &SubscriptionServer{subs: subs, devices: devices}
}

func (s *SubscriptionServer) GetSubscriptions(ctx context.Context, req *subpb.GetSubscriptionsRequest) (*subpb.GetSubscriptionsResponse, error) {
	if req.ProjectId == "" {
		return nil, status.Error(codes.InvalidArgument, "project_id is required")
	}

	rows, err := s.subs.ListByProjectID(ctx, req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query subscriptions: %v", err)
	}

	subs := make([]*subpb.Subscription, 0, len(rows))
	for _, row := range rows {
		tokens, err := s.deviceTokens(ctx, row.UserID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "query devices: %v", err)
		}

		subs = append(subs, &subpb.Subscription{
			Id:              strconv.FormatInt(row.ID, 10),
			ProjectId:       req.ProjectId,
			PushEnabled:     row.PushEnabled,
			MinAlertLevel:   row.MinAlertLevel,
			QuietHoursStart: derefString(row.QuietHoursStart),
			QuietHoursEnd:   derefString(row.QuietHoursEnd),
			FcmTokens:       tokens,
		})
	}

	return &subpb.GetSubscriptionsResponse{Subscriptions: subs}, nil
}

func (s *SubscriptionServer) deviceTokens(ctx context.Context, userID int64) ([]string, error) {
	devices, err := s.devices.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	tokens := make([]string, 0, len(devices))
	for _, d := range devices {
		tokens = append(tokens, d.DeviceToken)
	}
	return tokens, nil
}

func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
