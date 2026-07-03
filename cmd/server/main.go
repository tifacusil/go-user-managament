package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	googlegrpc "google.golang.org/grpc"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"

	"github.com/tifacusil/go-user-management/internal/client"
	appseed "github.com/tifacusil/go-user-management/internal/seed"
	"github.com/tifacusil/go-user-management/internal/config"
	"github.com/tifacusil/go-user-management/internal/discovery"
	"github.com/tifacusil/go-user-management/internal/docs"
	appgrpc "github.com/tifacusil/go-user-management/internal/grpc"
	subpb "github.com/tifacusil/go-user-management/internal/grpc/usermanagementpb"
	"github.com/tifacusil/go-user-management/internal/handler"
	appmiddleware "github.com/tifacusil/go-user-management/internal/middleware"
	"github.com/tifacusil/go-user-management/internal/observability"
	"github.com/tifacusil/go-user-management/internal/repository"
	"github.com/tifacusil/go-user-management/internal/service"
)

func main() {
	cfg := config.Load()

	instanceID := cfg.InstanceID
	if instanceID == "" {
		instanceID = cfg.ServiceName + "-" + uuid.NewString()
	}

	logger := observability.NewLogger(cfg.ServiceName, instanceID)
	slog.SetDefault(logger)

	if err := run(cfg, logger, instanceID); err != nil {
		logger.Error("fatal", "event", "startup_failed", "err", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, logger *slog.Logger, instanceID string) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("running migrations", "event", "db_migrate")
	if err := config.MigrateUp(cfg); err != nil {
		return err
	}

	logger.Info("connecting to mysql", "event", "db_connect", "host", cfg.DBHost, "db", cfg.DBName, "user", cfg.DBUser)
	db, err := config.Connect(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Warn("db close", "err", err)
		}
	}()

	userRepo := repository.NewUserRepository(db)
	subRepo := repository.NewSubscriptionRepository(db)
	deviceRepo := repository.NewDeviceRepository(db)
	projectClient := client.NewProjectClient(cfg.ProjectManagementURL)

	authSvc := service.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTExpiryHours)

	if cfg.SeedOnStartup {
		appseed.Run(ctx, authSvc, userRepo, subRepo, appseed.Options{
			AdminPassword: cfg.SeedAdminPassword,
			UserPassword:  cfg.SeedUserPassword,
		})
	}
	userSvc := service.NewUserService(userRepo)
	subSvc := service.NewSubscriptionService(subRepo, projectClient)
	deviceSvc := service.NewDeviceService(deviceRepo)

	authH := handler.NewAuthHandler(authSvc)
	userH := handler.NewUserHandler(userSvc)
	subH := handler.NewSubscriptionHandler(subSvc)
	deviceH := handler.NewDeviceHandler(deviceSvc)

	metrics := observability.NewMetrics()

	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(observability.RequestLogger(logger))
	r.Use(observability.AccessLogger(cfg.HealthPath))
	r.Use(metrics.Middleware)
	r.Use(chimiddleware.Recoverer)

	r.Handle("/metrics", metrics.Handler())

	// Swagger UI + spec: GET /api/v1/users/docs (UI), /api/v1/users/docs/openapi.yaml.
	// El gateway nginx enruta /api/v1/users/ SIN rewrite, así que el path público
	// coincide con el montaje aquí.
	r.Mount("/api/v1/users/docs", docs.Routes())

	r.Route("/api/v1/users", func(r chi.Router) {
		r.Post("/register", authH.Register)
		r.Post("/login", authH.Login)

		r.Group(func(r chi.Router) {
			r.Use(appmiddleware.JWT([]byte(cfg.JWTSecret)))
			r.Post("/logout", deviceH.Logout)
			r.Get("/{id}", userH.GetUser)
			r.Put("/{id}", userH.UpdateUser)
			r.Post("/{id}/subscriptions", subH.Create)
			r.Get("/{id}/subscriptions", subH.List)
			r.Post("/{id}/devices", deviceH.Register)
			r.Get("/{id}/devices", deviceH.List)
			r.Delete("/{id}/devices/{deviceId}", deviceH.Revoke)
		})
	})

	r.Get(cfg.HealthPath, func(w http.ResponseWriter, req *http.Request) {
		log := observability.FromContext(req.Context())
		body := map[string]string{"service": cfg.ServiceName, "status": "UP", "db": "UP"}
		status := http.StatusOK
		if err := db.PingContext(req.Context()); err != nil {
			log.Warn("health degraded", "event", "health_check", "err", err)
			status = http.StatusServiceUnavailable
			body = map[string]string{"service": cfg.ServiceName, "status": "DOWN", "db": "DOWN"}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	})

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	grpcSrv := googlegrpc.NewServer(
		googlegrpc.ChainUnaryInterceptor(
			observability.UnaryServerLogger(logger),
			metrics.UnaryServerInterceptor(),
		),
	)
	subpb.RegisterSubscriptionServiceServer(grpcSrv, appgrpc.NewSubscriptionServer(subRepo, deviceRepo))

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("http listening", "event", "http_start", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()
	go func() {
		lis, err := net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			serveErr <- err
			return
		}
		logger.Info("grpc listening", "event", "grpc_start", "addr", cfg.GRPCAddr)
		if err := grpcSrv.Serve(lis); err != nil {
			serveErr <- err
		}
	}()

	reg := registerConsul(cfg, logger, instanceID)
	if reg != nil {
		defer func() {
			if err := reg.Deregister(); err != nil {
				logger.Warn("consul deregister failed", "event", "consul_deregister", "err", err)
			} else {
				logger.Info("consul deregistered", "event", "consul_deregister")
			}
		}()
	}

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received", "event", "shutdown_begin")
	}

	grpcSrv.GracefulStop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	logger.Info("shutdown complete", "event", "shutdown_done")
	return nil
}

func registerConsul(cfg config.Config, logger *slog.Logger, instanceID string) *discovery.Registration {
	if !cfg.ConsulEnabled {
		logger.Info("consul disabled", "event", "consul_skip")
		return nil
	}

	port, err := portFromAddr(cfg.HTTPAddr)
	if err != nil {
		logger.Warn("cannot derive port for consul", "event", "consul_register", "err", err)
		return nil
	}

	healthURL := "http://" + net.JoinHostPort(cfg.AdvertiseHost, strconv.Itoa(port)) + cfg.HealthPath
	reg, err := discovery.Register(discovery.Options{
		ConsulAddr:  cfg.ConsulAddr,
		Token:       cfg.ConsulToken,
		ServiceName: cfg.ServiceName,
		InstanceID:  instanceID,
		Address:     cfg.AdvertiseHost,
		Port:        port,
		HealthURL:   healthURL,
		Interval:    cfg.ConsulHealthInterval,
		Tags:        []string{"version-" + cfg.ServiceVersion, "lang-go", "env-" + cfg.Env},
	})
	if err != nil {
		logger.Warn("consul registration failed; continuing", "event", "consul_register", "err", err)
		return nil
	}
	logger.Info("consul registered", "event", "consul_register", "consul", cfg.ConsulAddr, "id", instanceID)
	return reg
}

func portFromAddr(addr string) (int, error) {
	_, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(portStr)
}
