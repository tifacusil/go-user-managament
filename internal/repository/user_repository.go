package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/tifacusil/go-user-management/internal/domain"
)

var ErrNotFound = errors.New("not found")
var ErrConflict = errors.New("already exists")

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, u *domain.User) error {
	const q = `INSERT INTO users (username, email, phone_e164, password_hash, role, status)
	           VALUES (:username, :email, :phone_e164, :password_hash, :role, :status)`
	res, err := r.db.NamedExecContext(ctx, q, u)
	if err != nil {
		if isDuplicate(err) {
			return fmt.Errorf("%w: username or email", ErrConflict)
		}
		return fmt.Errorf("insert user: %w", err)
	}
	id, _ := res.LastInsertId()
	u.ID = id
	return nil
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	var u domain.User
	err := r.db.GetContext(ctx, &u, `SELECT * FROM users WHERE username = ? LIMIT 1`, username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by username: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) FindByID(ctx context.Context, id int64) (*domain.User, error) {
	var u domain.User
	err := r.db.GetContext(ctx, &u, `SELECT * FROM users WHERE id = ? LIMIT 1`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return &u, nil
}

func (r *UserRepository) Update(ctx context.Context, u *domain.User) error {
	const q = `UPDATE users SET email = :email, phone_e164 = :phone_e164, password_hash = :password_hash,
	            role = :role, status = :status WHERE id = :id`
	_, err := r.db.NamedExecContext(ctx, q, u)
	if err != nil {
		if isDuplicate(err) {
			return fmt.Errorf("%w: email", ErrConflict)
		}
		return fmt.Errorf("update user: %w", err)
	}
	return nil
}

func isDuplicate(err error) bool {
	if err == nil {
		return false
	}
	return contains(err.Error(), "Duplicate entry") || contains(err.Error(), "1062")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsRune(s, sub))
}

func containsRune(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
