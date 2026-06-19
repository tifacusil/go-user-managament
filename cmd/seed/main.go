package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/tifacusil/go-user-management/internal/config"
	"github.com/tifacusil/go-user-management/internal/repository"
	"github.com/tifacusil/go-user-management/internal/seed"
	"github.com/tifacusil/go-user-management/internal/service"
)

func main() {
	cfg := config.Load()

	adminPass := requireEnv("SEED_ADMIN_PASSWORD")
	userPass := requireEnv("SEED_USER_PASSWORD")

	ctx := context.Background()
	db, err := config.Connect(ctx, cfg)
	if err != nil {
		slog.Error("db connect failed", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Warn("db close", "err", err)
		}
	}()

	userRepo := repository.NewUserRepository(db)
	subRepo := repository.NewSubscriptionRepository(db)
	authSvc := service.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTExpiryHours)

	seed.Run(ctx, authSvc, userRepo, subRepo, seed.Options{
		AdminPassword: adminPass,
		UserPassword:  userPass,
	})
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("missing required env var", "key", key)
		os.Exit(1)
	}
	return v
}
