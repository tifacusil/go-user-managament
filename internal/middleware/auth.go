package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const claimsKey contextKey = "jwt_claims"

func JWT(secret []byte) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw := r.Header.Get("Authorization")
			if !strings.HasPrefix(raw, "Bearer ") {
				http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
				return
			}

			tok, err := jwt.Parse(strings.TrimPrefix(raw, "Bearer "), func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return secret, nil
			}, jwt.WithExpirationRequired())

			if err != nil || !tok.Valid {
				http.Error(w, `{"error":"invalid or expired token"}`, http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsKey, tok.Claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func ClaimsFromContext(ctx context.Context) jwt.MapClaims {
	c, _ := ctx.Value(claimsKey).(jwt.MapClaims)
	return c
}

func UserIDFromContext(ctx context.Context) int64 {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return 0
	}
	switch v := claims["sub"].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	}
	return 0
}

func RoleFromContext(ctx context.Context) string {
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		return ""
	}
	r, _ := claims["role"].(string)
	return r
}
