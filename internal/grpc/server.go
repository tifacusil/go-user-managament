package grpc

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/tifacusil/go-user-management/internal/grpc/alertrecipientpb"
	"github.com/tifacusil/go-user-management/internal/repository"
)

// alertLevelRank maps alert level strings to an integer for comparison.
var alertLevelRank = map[string]int{
	"blue":   1,
	"orange": 2,
	"red":    3,
}

type AlertRecipientServer struct {
	pb.UnimplementedAlertRecipientServiceServer
	subs    *repository.SubscriptionRepository
	devices *repository.DeviceRepository
}

func NewAlertRecipientServer(
	subs *repository.SubscriptionRepository,
	devices *repository.DeviceRepository,
) *AlertRecipientServer {
	return &AlertRecipientServer{subs: subs, devices: devices}
}

func (s *AlertRecipientServer) GetRecipients(ctx context.Context, req *pb.GetRecipientsRequest) (*pb.GetRecipientsResponse, error) {
	if req.ProjectId == "" || req.AlertLevel == "" {
		return nil, status.Error(codes.InvalidArgument, "project_id and alert_level are required")
	}

	incomingRank, ok := alertLevelRank[req.AlertLevel]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown alert_level: %s", req.AlertLevel)
	}

	allSubs, err := s.subs.ListByProjectID(ctx, req.ProjectId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query subscriptions: %v", err)
	}

	now := time.Now().UTC()
	var recipients []*pb.Recipient

	for _, row := range allSubs {
		// Skip if subscription threshold is above incoming level.
		if alertLevelRank[row.MinAlertLevel] > incomingRank {
			continue
		}

		// Red alerts bypass quiet hours entirely.
		if req.AlertLevel != "red" && isQuietHour(now, row.QuietHoursStart, row.QuietHoursEnd) {
			continue
		}

		tokens, err := s.deviceTokens(ctx, row.UserID)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "query devices: %v", err)
		}

		recipients = append(recipients, &pb.Recipient{
			UserId:       row.UserID,
			Email:        row.Email,
			PhoneE164:    row.PhoneE164,
			SmsEnabled:   row.SMSEnabled,
			PushEnabled:  row.PushEnabled,
			EmailEnabled: row.EmailEnabled,
			DeviceTokens: tokens,
		})
	}

	return &pb.GetRecipientsResponse{Recipients: recipients}, nil
}

func (s *AlertRecipientServer) deviceTokens(ctx context.Context, userID int64) ([]string, error) {
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

// isQuietHour returns true if now falls within [start, end) with midnight-crossing support.
func isQuietHour(now time.Time, start, end *string) bool {
	if start == nil || end == nil {
		return false
	}
	s, err1 := parseHHMM(*start)
	e, err2 := parseHHMM(*end)
	if err1 != nil || err2 != nil {
		return false
	}

	nowMins := now.Hour()*60 + now.Minute()

	if s <= e {
		// Same-day window e.g. 08:00–20:00
		return nowMins >= s && nowMins < e
	}
	// Midnight-crossing window e.g. 22:00–06:00
	return nowMins >= s || nowMins < e
}

func parseHHMM(t string) (int, error) {
	var h, m int
	_, err := parseTime(t, &h, &m)
	return h*60 + m, err
}

func parseTime(t string, h, m *int) (int, error) {
	if len(t) != 5 || t[2] != ':' {
		return 0, status.Errorf(codes.Internal, "invalid time format: %s", t)
	}
	*h = int(t[0]-'0')*10 + int(t[1]-'0')
	*m = int(t[3]-'0')*10 + int(t[4]-'0')
	return 0, nil
}
