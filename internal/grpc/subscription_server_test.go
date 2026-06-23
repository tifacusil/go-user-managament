package grpc

import (
	"context"
	"errors"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/tifacusil/go-user-management/internal/domain"
	subpb "github.com/tifacusil/go-user-management/internal/grpc/usermanagementpb"
)

type fakeSubs struct {
	rows []domain.RecipientRow
	err  error
}

func (f fakeSubs) ListByProjectID(ctx context.Context, projectID string) ([]domain.RecipientRow, error) {
	return f.rows, f.err
}

type fakeDevices struct {
	byUser map[int64][]domain.UserDevice
	err    error
}

func (f fakeDevices) ListActiveByUserID(ctx context.Context, userID int64) ([]domain.UserDevice, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byUser[userID], nil
}

// newTestClient wires the SubscriptionServer behind an in-memory bufconn gRPC
// server and returns the generated client plus a cleanup func.
func newTestClient(t *testing.T, srvImpl *SubscriptionServer) subpb.SubscriptionServiceClient {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	gsrv := grpc.NewServer()
	subpb.RegisterSubscriptionServiceServer(gsrv, srvImpl)
	go func() { _ = gsrv.Serve(lis) }()

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() {
		conn.Close()
		gsrv.Stop()
	})
	return subpb.NewSubscriptionServiceClient(conn)
}

func ptr(s string) *string { return &s }

func TestGetSubscriptions_MapsRowsAndTokens(t *testing.T) {
	srv := NewSubscriptionServer(
		fakeSubs{rows: []domain.RecipientRow{
			{ID: 7, UserID: 100, MinAlertLevel: "orange", PushEnabled: true,
				QuietHoursStart: ptr("22:00:00"), QuietHoursEnd: ptr("06:00:00")},
			{ID: 8, UserID: 200, MinAlertLevel: "blue", PushEnabled: false},
		}},
		fakeDevices{byUser: map[int64][]domain.UserDevice{
			100: {{DeviceToken: "tok-a"}, {DeviceToken: "tok-b"}},
			// user 200 has no active devices
		}},
	)
	client := newTestClient(t, srv)

	resp, err := client.GetSubscriptions(context.Background(),
		&subpb.GetSubscriptionsRequest{ProjectId: "proj-001"})
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}

	subs := resp.GetSubscriptions()
	if len(subs) != 2 {
		t.Fatalf("subscriptions = %d, want 2", len(subs))
	}

	s0 := subs[0]
	if s0.GetId() != "7" {
		t.Errorf("id = %q, want \"7\" (int64 stringified)", s0.GetId())
	}
	if s0.GetProjectId() != "proj-001" {
		t.Errorf("project_id = %q, want proj-001 (echoed from request)", s0.GetProjectId())
	}
	if !s0.GetPushEnabled() || s0.GetMinAlertLevel() != "orange" {
		t.Errorf("push/level = %v/%q, want true/orange", s0.GetPushEnabled(), s0.GetMinAlertLevel())
	}
	if s0.GetQuietHoursStart() != "22:00:00" || s0.GetQuietHoursEnd() != "06:00:00" {
		t.Errorf("quiet hours = %q-%q", s0.GetQuietHoursStart(), s0.GetQuietHoursEnd())
	}
	if got := s0.GetFcmTokens(); len(got) != 2 || got[0] != "tok-a" || got[1] != "tok-b" {
		t.Errorf("fcm_tokens = %v, want [tok-a tok-b]", got)
	}

	s1 := subs[1]
	if s1.GetPushEnabled() {
		t.Error("s1 push_enabled should be false (server returns raw, no filtering)")
	}
	// nil quiet-hour pointers must map to empty strings, not panic.
	if s1.GetQuietHoursStart() != "" || s1.GetQuietHoursEnd() != "" {
		t.Errorf("nil quiet hours = %q-%q, want empty", s1.GetQuietHoursStart(), s1.GetQuietHoursEnd())
	}
	if len(s1.GetFcmTokens()) != 0 {
		t.Errorf("s1 fcm_tokens = %v, want empty", s1.GetFcmTokens())
	}
}

func TestGetSubscriptions_EmptyProjectID(t *testing.T) {
	client := newTestClient(t, NewSubscriptionServer(fakeSubs{}, fakeDevices{}))

	_, err := client.GetSubscriptions(context.Background(),
		&subpb.GetSubscriptionsRequest{ProjectId: ""})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestGetSubscriptions_RepoError(t *testing.T) {
	client := newTestClient(t, NewSubscriptionServer(
		fakeSubs{err: errors.New("db down")}, fakeDevices{}))

	_, err := client.GetSubscriptions(context.Background(),
		&subpb.GetSubscriptionsRequest{ProjectId: "proj-001"})
	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal", status.Code(err))
	}
}

func TestGetSubscriptions_NoSubscriptions(t *testing.T) {
	client := newTestClient(t, NewSubscriptionServer(fakeSubs{rows: nil}, fakeDevices{}))

	resp, err := client.GetSubscriptions(context.Background(),
		&subpb.GetSubscriptionsRequest{ProjectId: "proj-001"})
	if err != nil {
		t.Fatalf("GetSubscriptions: %v", err)
	}
	if len(resp.GetSubscriptions()) != 0 {
		t.Fatalf("subscriptions = %d, want 0", len(resp.GetSubscriptions()))
	}
}
