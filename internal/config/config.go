package config

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/joho/godotenv"
	"github.com/sethvargo/go-retry"

	_ "github.com/go-sql-driver/mysql"
)

type Config struct {
	ServiceName    string
	ServiceVersion string
	Env            string

	InstanceID    string
	HTTPAddr      string
	AdvertiseHost string
	HealthPath    string

	DBHost         string
	DBPort         string
	DBUser         string
	DBPass         string
	DBName         string
	DBRootUser     string
	DBRootPass     string
	DBMaxOpenConns int
	DBMaxIdleConns int

	ConsulEnabled        bool
	ConsulAddr           string
	ConsulToken          string
	ConsulHealthInterval string

	JWTSecret          string
	JWTExpiryHours     int

	ProjectManagementURL string

	GRPCAddr string
}

func Load() Config {
	_ = godotenv.Load()

	return Config{
		ServiceName:    env("SERVICE_NAME", "user-management-service"),
		ServiceVersion: env("SERVICE_VERSION", "0.1.0"),
		Env:            env("ENV", "dev"),
		InstanceID:     os.Getenv("INSTANCE_ID"),

		HTTPAddr:      httpAddr(),
		AdvertiseHost: env("ADVERTISE_HOST", "127.0.0.1"),
		HealthPath:    env("HEALTH_PATH", "/health"),

		DBHost:         env("DB_HOST", "127.0.0.1"),
		DBPort:         env("DB_PORT", "3306"),
		DBUser:         env("DB_USER", "svc_user_mgmt"),
		DBPass:         env("DB_PASS", ""),
		DBName:         env("DB_NAME", "user_management"),
		DBRootUser:     env("DB_ROOT_USER", "root"),
		DBRootPass:     env("DB_ROOT_PASS", ""),
		DBMaxOpenConns: envInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns: envInt("DB_MAX_IDLE_CONNS", 25),

		ConsulEnabled:        envBool("CONSUL_ENABLED", true),
		ConsulAddr:           env("CONSUL_HOST", "localhost") + ":" + env("CONSUL_PORT", "8500"),
		ConsulToken:          env("CONSUL_TOKEN", ""),
		ConsulHealthInterval: env("CONSUL_HEALTH_INTERVAL", "10s"),

		JWTSecret:      env("JWT_SECRET", ""),
		JWTExpiryHours: envInt("JWT_EXPIRY_HOURS", 24),

		ProjectManagementURL: env("PROJECT_MANAGEMENT_URL", "http://project-management-1:8080"),

		GRPCAddr: env("GRPC_ADDR", ":9090"),
	}
}

func (c Config) DSN() string {
	return c.dsnFor(c.DBUser, c.DBPass, nil)
}

func (c Config) RootDSN() string {
	return c.dsnFor(c.DBRootUser, c.DBRootPass, map[string]string{"multiStatements": "true"})
}

func (c Config) dsnFor(user, pass string, extra map[string]string) string {
	cfg := mysql.NewConfig()
	cfg.User = user
	cfg.Passwd = pass
	cfg.Net = "tcp"
	cfg.Addr = c.DBHost + ":" + c.DBPort
	cfg.DBName = c.DBName
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	for k, v := range extra {
		cfg.Params[k] = v
	}
	return cfg.FormatDSN()
}

func Connect(ctx context.Context, c Config) (*sqlx.DB, error) {
	db, err := sqlx.Open("mysql", c.DSN())
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	db.SetMaxOpenConns(c.DBMaxOpenConns)
	db.SetMaxIdleConns(c.DBMaxIdleConns)
	db.SetConnMaxLifetime(5 * time.Minute)

	b := retry.WithMaxDuration(30*time.Second, retry.WithCappedDuration(10*time.Second, retry.NewExponential(time.Second)))
	if err := retry.Do(ctx, b, func(ctx context.Context) error {
		if err := db.PingContext(ctx); err != nil {
			return retry.RetryableError(err)
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	return db, nil
}

func httpAddr() string {
	if a := os.Getenv("HTTP_ADDR"); a != "" {
		return a
	}
	return ":" + env("HTTP_PORT", "8080")
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}
