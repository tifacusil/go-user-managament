//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	authSvc := service.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTExpiryHours)
	userSvc := service.NewUserService(userRepo)
	authH := handler.NewAuthHandler(authSvc)
	userH := handler.NewUserHandler(userSvc)

	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Route("/api/v1/users", func(r chi.Router) {
		r.Post("/register", authH.Register)
		r.Post("/login", authH.Login)
		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.JWT([]byte(cfg.JWTSecret)))
			r.Get("/{id}", userH.GetUser)
			r.Put("/{id}", userH.UpdateUser)
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
