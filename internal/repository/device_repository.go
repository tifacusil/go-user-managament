package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/tifacusil/go-user-management/internal/domain"
)

type DeviceRepository struct {
	db *sqlx.DB
}

func NewDeviceRepository(db *sqlx.DB) *DeviceRepository {
	return &DeviceRepository{db: db}
}

func (r *DeviceRepository) FindByToken(ctx context.Context, token string) (*domain.UserDevice, error) {
	var d domain.UserDevice
	err := r.db.GetContext(ctx, &d, `SELECT * FROM user_device WHERE device_token = ? LIMIT 1`, token)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find device by token: %w", err)
	}
	return &d, nil
}

func (r *DeviceRepository) FindByID(ctx context.Context, id int64) (*domain.UserDevice, error) {
	var d domain.UserDevice
	err := r.db.GetContext(ctx, &d, `SELECT * FROM user_device WHERE id = ? LIMIT 1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find device by id: %w", err)
	}
	return &d, nil
}

func (r *DeviceRepository) ListActiveByUserID(ctx context.Context, userID int64) ([]domain.UserDevice, error) {
	var devices []domain.UserDevice
	err := r.db.SelectContext(ctx, &devices,
		`SELECT * FROM user_device WHERE user_id = ? AND status = 'active' ORDER BY id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}
	return devices, nil
}

// Upsert inserts a new device row or updates user_id, app_version, last_seen_at, status
// when the token already exists (ownership transfer).
func (r *DeviceRepository) Upsert(ctx context.Context, d *domain.UserDevice) error {
	const q = `INSERT INTO user_device (user_id, device_token, platform, app_version, last_seen_at, status)
	           VALUES (:user_id, :device_token, :platform, :app_version, NOW(), :status)
	           ON DUPLICATE KEY UPDATE
	               user_id     = VALUES(user_id),
	               app_version = VALUES(app_version),
	               last_seen_at = NOW(),
	               status      = VALUES(status)`
	res, err := r.db.NamedExecContext(ctx, q, d)
	if err != nil {
		return fmt.Errorf("upsert device: %w", err)
	}
	id, _ := res.LastInsertId()
	if id != 0 {
		d.ID = id
	}
	return nil
}

func (r *DeviceRepository) Revoke(ctx context.Context, id, userID int64) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE user_device SET status = 'revoked' WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return fmt.Errorf("revoke device: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}
