//go:build contract

// Package cdct contains Consumer-Driven Contract Tests (CDCT) for the gRPC
// AlertRecipientService. The early-warning service is the consumer; this file
// defines the contract it expects from user-management (the provider).
//
// Run consumer tests:   go test -tags=contract ./test/cdct/... -run TestConsumer
// Run provider verify:  go test -tags=contract ./test/cdct/... -run TestProvider
//
// Prerequisites:
//   - pact-go install (downloads the native FFI lib): go run github.com/pact-foundation/pact-go/v2 install
//   - Docker (for provider verification)
package cdct

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/pact-foundation/pact-go/v2/provider"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/tifacusil/go-user-management/internal/grpc/alertrecipientpb"
)

const (
	consumer_name = "early-warning-service"
	provider_name = "user-management-service"
)

func pactDir() string {
	dir, _ := filepath.Abs("pacts")
	return dir
}

// TestConsumerGetRecipients defines the contract from the early-warning
// service's perspective: given a project with an active subscriber,
// GetRecipients must return at least one recipient with the expected fields.
func TestConsumerGetRecipients(t *testing.T) {
	mockProvider, err := consumer.NewV4Pact(consumer.MockHTTPProviderConfig{
		Consumer: consumer_name,
		Provider: provider_name,
		PactDir:  pactDir(),
	})
	if err != nil {
		t.Fatalf("create pact: %v", err)
	}

	mockProvider.
		AddInteraction().
		Given("project proj-001 has one active subscriber at blue level").
		UponReceiving("a GetRecipients request for proj-001 at blue alert level").
		WithRequest("POST", "/alertrecipient.AlertRecipientService/GetRecipients").
		WithJSONBody(matchers.Map{
			"projectId":  matchers.String("proj-001"),
			"alertLevel": matchers.String("blue"),
		}).
		WillRespondWith(200).
		WithJSONBody(matchers.Map{
			"recipients": matchers.EachLike(matchers.Map{
				"userId":       matchers.Integer(1),
				"email":        matchers.String("user@example.com"),
				"smsEnabled":   matchers.Boolean(true),
				"pushEnabled":  matchers.Boolean(true),
				"emailEnabled": matchers.Boolean(true),
			}, 1),
		})

	err = mockProvider.ExecuteTest(t, func(config consumer.MockServerConfig) error {
		addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return err
		}
		defer conn.Close()

		client := pb.NewAlertRecipientServiceClient(conn)
		resp, err := client.GetRecipients(context.Background(), &pb.GetRecipientsRequest{
			ProjectId:  "proj-001",
			AlertLevel: "blue",
		})
		if err != nil {
			return fmt.Errorf("GetRecipients: %w", err)
		}
		if len(resp.Recipients) == 0 {
			return fmt.Errorf("expected at least one recipient, got 0")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("pact test: %v", err)
	}
}

// TestProviderVerification verifies that the running user-management service
// honours the contracts stored in the pacts/ directory.
func TestProviderVerification(t *testing.T) {
	providerAddr := os.Getenv("PROVIDER_GRPC_ADDR")
	if providerAddr == "" {
		providerAddr = "localhost:9090"
	}

	verifier := provider.NewVerifier()
	err := verifier.VerifyProvider(t, provider.VerifyRequest{
		ProviderBaseURL:            "grpc://" + providerAddr,
		Provider:                   provider_name,
		PactFiles:                  []string{filepath.Join(pactDir(), consumer_name+"-"+provider_name+".json")},
		StateHandlers: provider.StateHandlers{
			"project proj-001 has one active subscriber at blue level": func(setup bool, s provider.ProviderStateV3) (provider.ProviderStateV3Response, error) {
				// In a full setup this would seed the DB via the test helper API.
				// For the thesis demo the state is pre-seeded manually.
				return provider.ProviderStateV3Response{}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("provider verification: %v", err)
	}
}
