package discovery

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Registration struct {
	httpc      *http.Client
	baseURL    string
	token      string
	instanceID string
}

type Options struct {
	ConsulAddr  string
	Token       string
	ServiceName string
	InstanceID  string
	Address     string
	Port        int
	HealthURL   string
	Interval    string
	Tags        []string
}

type agentCheck struct {
	HTTP                           string `json:"HTTP"`
	Interval                       string `json:"Interval"`
	Timeout                        string `json:"Timeout"`
	DeregisterCriticalServiceAfter string `json:"DeregisterCriticalServiceAfter"`
}

type agentService struct {
	ID      string     `json:"ID"`
	Name    string     `json:"Name"`
	Address string     `json:"Address"`
	Port    int        `json:"Port"`
	Tags    []string   `json:"Tags"`
	Check   agentCheck `json:"Check"`
}

func Register(opts Options) (*Registration, error) {
	body := agentService{
		ID:      opts.InstanceID,
		Name:    opts.ServiceName,
		Address: opts.Address,
		Port:    opts.Port,
		Tags:    opts.Tags,
		Check: agentCheck{
			HTTP:                           opts.HealthURL,
			Interval:                       opts.Interval,
			Timeout:                        "5s",
			DeregisterCriticalServiceAfter: "1m",
		},
	}

	reg := &Registration{
		httpc:      &http.Client{Timeout: 5 * time.Second},
		baseURL:    "http://" + opts.ConsulAddr,
		token:      opts.Token,
		instanceID: opts.InstanceID,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := reg.put(ctx, "/v1/agent/service/register", body); err != nil {
		return nil, fmt.Errorf("consul register %s: %w", opts.InstanceID, err)
	}
	return reg, nil
}

func (r *Registration) Deregister() error {
	if r == nil || r.httpc == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := r.put(ctx, "/v1/agent/service/deregister/"+r.instanceID, nil); err != nil {
		return fmt.Errorf("consul deregister %s: %w", r.instanceID, err)
	}
	return nil
}

func (r *Registration) put(ctx context.Context, path string, payload any) error {
	var rdr io.Reader
	if payload != nil {
		buf, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		rdr = bytes.NewReader(buf)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, r.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		req.Header.Set("X-Consul-Token", r.token)
	}

	resp, err := r.httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("consul agent %s: status %d: %s", path, resp.StatusCode, bytes.TrimSpace(msg))
	}
	return nil
}
