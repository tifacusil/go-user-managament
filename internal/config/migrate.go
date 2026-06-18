package config

import (
	"database/sql"
	"fmt"

	"github.com/pressly/goose/v3"

	_ "github.com/go-sql-driver/mysql"

	"github.com/tifacusil/go-user-management/migrations"
)

func MigrateUp(c Config) error {
	db, err := sql.Open("mysql", c.RootDSN())
	if err != nil {
		return fmt.Errorf("open root conn: %w", err)
	}
	defer db.Close() //nolint:errcheck

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("mysql"); err != nil {
		return fmt.Errorf("goose dialect: %w", err)
	}
	if err := goose.Up(db, "."); err != nil {
		return fmt.Errorf("goose up: %w", err)
	}
	return nil
}
