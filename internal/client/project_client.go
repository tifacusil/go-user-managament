package client

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sony/gobreaker/v2"
)

var ErrProjectNotFound = errors.New("project not found")
var ErrCircuitOpen = errors.New("project-management service unavailable")

type ProjectClient struct {
	baseURL string
	http    *http.Client
	cb      *gobreaker.CircuitBreaker[struct{}]
}

func NewProjectClient(baseURL string) *ProjectClient {
	cb := gobreaker.NewCircuitBreaker[struct{}](gobreaker.Settings{
		Name:        "project-management",
		MaxRequests: 3,                // half-open: allow 3 probes
		Interval:    60 * time.Second, // reset counts every 60s in closed state
		Timeout:     30 * time.Second, // stay open for 30s before half-open
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	})

	return &ProjectClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
		cb:      cb,
	}
}

// ProjectExists returns nil if the project exists, ErrProjectNotFound if it doesn't,
// or ErrCircuitOpen if the circuit breaker is open.
func (c *ProjectClient) ProjectExists(ctx context.Context, projectID string) error {
	_, err := c.cb.Execute(func() (struct{}, error) {
		return struct{}{}, c.checkProject(ctx, projectID)
	})

	if err != nil {
		if errors.Is(err, ErrProjectNotFound) {
			return ErrProjectNotFound
		}
		if errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests) {
			return ErrCircuitOpen
		}
		return fmt.Errorf("project check: %w", err)
	}
	return nil
}

func (c *ProjectClient) checkProject(ctx context.Context, projectID string) error {
	url := fmt.Sprintf("%s/api/v1/projects/%s", c.baseURL, projectID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return ErrProjectNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}
