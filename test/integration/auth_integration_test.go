//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/testcontainers/testcontainers-go/modules/mysql"

	"github.com/tifacusil/go-user-management/internal/config"
	"github.com/tifacusil/go-user-management/internal/handler"
	appmiddleware "github.com/tifacusil/go-user-management/internal/middleware"
	"github.com/tifacusil/go-user-management/internal/repository"
	"github.com/tifacusil/go-user-management/internal/service"
)

func setupTestServer(t *testing.T) (*httptest.Server, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := mysql.Run(ctx,
		"mysql:8.0",
		mysql.WithDatabase("user_management"),
		mysql.WithUsername("root"),
		mysql.WithPassword("testpass"),
	)
	if err != nil {
		t.Fatalf("start mysql container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	port, err := container.MappedPort(ctx, "3306")
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{
		ServiceName:    "test",
		DBHost:         host,
		DBPort:         port.Port(),
		DBUser:         "root",
		DBPass:         "testpass",
		DBRootUser:     "root",
		DBRootPass:     "testpass",
		DBName:         "user_management",
		DBMaxOpenConns: 5,
		DBMaxIdleConns: 5,
		JWTSecret:      "test-integration-secret",
		JWTExpiryHours: 1,
	}

	if err := config.MigrateUp(cfg); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	db, err := config.Connect(ctx, cfg)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}

	userRepo := repository.NewUserRepository(db)
	deviceRepo := repository.NewDeviceRepository(db)
	authSvc := service.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTExpiryHours)
	userSvc := service.NewUserService(userRepo)
	deviceSvc := service.NewDeviceService(deviceRepo)
	authH := handler.NewAuthHandler(authSvc)
	userH := handler.NewUserHandler(userSvc)
	deviceH := handler.NewDeviceHandler(deviceSvc)

	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Route("/api/v1/users", func(r chi.Router) {
		r.Post("/register", authH.Register)
		r.Post("/login", authH.Login)
		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.JWT([]byte(cfg.JWTSecret)))
			r.Post("/logout", deviceH.Logout)
			r.Get("/{id}", userH.GetUser)
			r.Put("/{id}", userH.UpdateUser)
			r.Post("/{id}/devices", deviceH.Register)
			r.Get("/{id}/devices", deviceH.List)
		})
	})

	srv := httptest.NewServer(r)
	cleanup := func() {
		srv.Close()
		db.Close()
		container.Terminate(ctx)
	}
	return srv, cleanup
}

func TestRegisterAndLogin(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Register
	body := `{"username":"alice","email":"alice@example.com","password":"password123"}`
	resp, err := http.Post(srv.URL+"/api/v1/users/register", "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register: got %d, want 201", resp.StatusCode)
	}

	var user map[string]any
	json.NewDecoder(resp.Body).Decode(&user)
	if user["username"] != "alice" {
		t.Errorf("username = %v, want alice", user["username"])
	}
	if _, ok := user["password_hash"]; ok {
		t.Error("password_hash must not be in response")
	}

	// Login
	loginBody := `{"username":"alice","password":"password123"}`
	resp2, err := http.Post(srv.URL+"/api/v1/users/login", "application/json", bytes.NewBufferString(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("login: got %d, want 200", resp2.StatusCode)
	}

	var loginResp map[string]any
	json.NewDecoder(resp2.Body).Decode(&loginResp)
	token, ok := loginResp["token"].(string)
	if !ok || token == "" {
		t.Fatal("expected token in login response")
	}

	// Get own profile
	userID := int64(user["user_id"].(float64))
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/v1/users/%d", srv.URL, userID), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("get profile: got %d, want 200", resp3.StatusCode)
	}
}

func TestDuplicateRegistration(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	body := `{"username":"bob","email":"bob@example.com","password":"password123"}`
	http.Post(srv.URL+"/api/v1/users/register", "application/json", bytes.NewBufferString(body))

	resp, _ := http.Post(srv.URL+"/api/v1/users/register", "application/json", bytes.NewBufferString(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("duplicate register: got %d, want 409", resp.StatusCode)
	}
}

func TestWrongPassword(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	http.Post(srv.URL+"/api/v1/users/register", "application/json",
		bytes.NewBufferString(`{"username":"carol","email":"carol@example.com","password":"password123"}`))

	resp, _ := http.Post(srv.URL+"/api/v1/users/login", "application/json",
		bytes.NewBufferString(`{"username":"carol","password":"wrongpassword"}`))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("wrong password: got %d, want 401", resp.StatusCode)
	}
}

func TestForbiddenAccessOtherUser(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Register two users and login as alice
	http.Post(srv.URL+"/api/v1/users/register", "application/json",
		bytes.NewBufferString(`{"username":"alice2","email":"alice2@example.com","password":"password123"}`))
	http.Post(srv.URL+"/api/v1/users/register", "application/json",
		bytes.NewBufferString(`{"username":"bob2","email":"bob2@example.com","password":"password123"}`))

	loginResp, _ := http.Post(srv.URL+"/api/v1/users/login", "application/json",
		bytes.NewBufferString(`{"username":"alice2","password":"password123"}`))
	var lr map[string]any
	json.NewDecoder(loginResp.Body).Decode(&lr)
	loginResp.Body.Close()
	token := lr["token"].(string)

	// Try to access user ID 2 as user ID 1
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/v1/users/2", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, _ := http.DefaultClient.Do(req)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cross-user access: got %d, want 403", resp.StatusCode)
	}
}

func TestLogoutRevokesDevices(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Register + login.
	http.Post(srv.URL+"/api/v1/users/register", "application/json",
		bytes.NewBufferString(`{"username":"dave","email":"dave@example.com","password":"password123"}`))
	loginResp, _ := http.Post(srv.URL+"/api/v1/users/login", "application/json",
		bytes.NewBufferString(`{"username":"dave","password":"password123"}`))
	var lr map[string]any
	json.NewDecoder(loginResp.Body).Decode(&lr)
	loginResp.Body.Close()
	token := lr["token"].(string)
	userID := int64(lr["user_id"].(float64))

	authed := func(method, path, body string) *http.Response {
		var r io.Reader
		if body != "" {
			r = bytes.NewBufferString(body)
		}
		req, _ := http.NewRequest(method, srv.URL+path, r)
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	// Register two devices.
	for _, tok := range []string{"fcm-token-a", "fcm-token-b"} {
		resp := authed(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/devices", userID),
			fmt.Sprintf(`{"device_token":%q,"platform":"android"}`, tok))
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			t.Fatalf("register device %s: got %d", tok, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// Two active devices present.
	if n := activeDeviceCount(t, authed, userID); n != 2 {
		t.Fatalf("before logout: got %d active devices, want 2", n)
	}

	// Logout → 204.
	resp := authed(http.MethodPost, "/api/v1/users/logout", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout: got %d, want 204", resp.StatusCode)
	}
	resp.Body.Close()

	// All devices revoked → none active.
	if n := activeDeviceCount(t, authed, userID); n != 0 {
		t.Errorf("after logout: got %d active devices, want 0", n)
	}

	// JWT still valid after logout (only the device tokens are revoked).
	resp = authed(http.MethodGet, fmt.Sprintf("/api/v1/users/%d", userID), "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("profile after logout: got %d, want 200 (JWT must stay valid)", resp.StatusCode)
	}
	resp.Body.Close()

	// Logout again is idempotent → still 204.
	resp = authed(http.MethodPost, "/api/v1/users/logout", "")
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("second logout: got %d, want 204 (idempotent)", resp.StatusCode)
	}
	resp.Body.Close()
}

func activeDeviceCount(t *testing.T, authed func(method, path, body string) *http.Response, userID int64) int {
	t.Helper()
	resp := authed(http.MethodGet, fmt.Sprintf("/api/v1/users/%d/devices", userID), "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list devices: got %d, want 200", resp.StatusCode)
	}
	var out struct {
		Devices []map[string]any `json:"devices"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	return len(out.Devices)
}
