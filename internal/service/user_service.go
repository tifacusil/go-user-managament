package service

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/tifacusil/go-user-management/internal/domain"
	"github.com/tifacusil/go-user-management/internal/repository"
)

type UserService struct {
	users *repository.UserRepository
}

func NewUserService(users *repository.UserRepository) *UserService {
	return &UserService{users: users}
}

type UpdateUserInput struct {
	Email     string `json:"email"      validate:"omitempty,email"`
	PhoneE164 string `json:"phone_e164" validate:"omitempty,e164"`
	// Password change: both fields must be provided together.
	CurrentPassword string `json:"current_password" validate:"omitempty,min=8,max=72"`
	NewPassword     string `json:"new_password"     validate:"omitempty,min=8,max=72"`
	// Admin-only fields.
	Role   string `json:"role"   validate:"omitempty,oneof=admin operator viewer"`
	Status string `json:"status" validate:"omitempty,oneof=active inactive"`
}

func (s *UserService) GetByID(ctx context.Context, id int64) (*domain.User, error) {
	return s.users.FindByID(ctx, id)
}

// Update applies allowed field changes for targetID.
// callerID / callerRole come from the JWT claims; only admins may change role/status.
func (s *UserService) Update(ctx context.Context, targetID, callerID int64, callerRole string, in UpdateUserInput) (*domain.User, error) {
	u, err := s.users.FindByID(ctx, targetID)
	if err != nil {
		return nil, err
	}

	if in.Email != "" {
		u.Email = in.Email
	}
	if in.PhoneE164 != "" {
		u.PhoneE164 = in.PhoneE164
	}

	if in.NewPassword != "" {
		if in.CurrentPassword == "" {
			return nil, fmt.Errorf("current_password is required to change password")
		}
		if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.CurrentPassword)); err != nil {
			return nil, ErrInvalidCredentials
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(in.NewPassword), 10)
		if err != nil {
			return nil, fmt.Errorf("hash password: %w", err)
		}
		u.PasswordHash = string(hash)
	}

	if callerRole == domain.RoleAdmin {
		if in.Role != "" {
			u.Role = in.Role
		}
		if in.Status != "" {
			u.Status = in.Status
		}
	}

	if err := s.users.Update(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}
