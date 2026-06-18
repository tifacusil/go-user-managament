package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tifacusil/go-user-management/internal/middleware"
)

var testSecret = []byte("test-secret-key")

func makeToken(t *testing.T, userID int64, role string, expired bool) string {
	t.Helper()
	exp := time.Now().Add(time.Hour)
	if expired {
		exp = time.Now().Add(-time.Hour)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  float64(userID),
		"role": role,
		"exp":  exp.Unix(),
	})
	signed, err := tok.SignedString(testSecret)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestJWTMiddleware(t *testing.T) {
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.JWT(testSecret)(sentinel)

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic abc123", http.StatusUnauthorized},
		{"malformed token", "Bearer not.a.token", http.StatusUnauthorized},
		{"expired token", "Bearer " + func() string {
			tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"sub": float64(1), "exp": time.Now().Add(-time.Hour).Unix(),
			}).SignedString(testSecret)
			return tok
		}(), http.StatusUnauthorized},
		{"wrong secret", "Bearer " + func() string {
			tok, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
				"sub": float64(1), "exp": time.Now().Add(time.Hour).Unix(),
			}).SignedString([]byte("wrong-secret"))
			return tok
		}(), http.StatusUnauthorized},
		{"valid token", "Bearer " + makeToken(t, 42, "viewer", false), http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
		})
	}
}

func TestClaimsFromContext(t *testing.T) {
	var capturedID int64
	var capturedRole string

	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = middleware.UserIDFromContext(r.Context())
		capturedRole = middleware.RoleFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := middleware.JWT(testSecret)(sentinel)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+makeToken(t, 99, "admin", false))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if capturedID != 99 {
		t.Errorf("UserIDFromContext = %d, want 99", capturedID)
	}
	if capturedRole != "admin" {
		t.Errorf("RoleFromContext = %q, want admin", capturedRole)
	}
}
