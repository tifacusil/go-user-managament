package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/tifacusil/go-user-management/internal/domain"
	"github.com/tifacusil/go-user-management/internal/repository"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserInactive       = errors.New("account is not active")
)

type AuthService struct {
	users          *repository.UserRepository
	jwtSecret      []byte
	jwtExpiryHours int
}

func NewAuthService(users *repository.UserRepository, jwtSecret string, jwtExpiryHours int) *AuthService {
	return &AuthService{
		users:          users,
		jwtSecret:      []byte(jwtSecret),
		jwtExpiryHours: jwtExpiryHours,
	}
}

type RegisterInput struct {
	Username  string `json:"username"   validate:"required,min=3,max=50,alphanum"`
	Email     string `json:"email"      validate:"required,email"`
	Password  string `json:"password"   validate:"required,min=8,max=72"`
	PhoneE164 string `json:"phone_e164" validate:"omitempty,e164"`
	Role      string `json:"role"       validate:"omitempty,oneof=admin operator viewer"`
}

type LoginInput struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type TokenResponse struct {
	Token     string    `json:"token"`
	UserID    int64     `json:"user_id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (s *AuthService) Register(ctx context.Context, in RegisterInput) (*domain.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), 10)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	role := in.Role
	if role == "" {
		role = domain.RoleViewer
	}

	u := &domain.User{
		Username:     in.Username,
		Email:        in.Email,
		PhoneE164:    in.PhoneE164,
		PasswordHash: string(hash),
		Role:         role,
		Status:       domain.StatusActive,
	}

	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *AuthService) Login(ctx context.Context, in LoginInput) (*TokenResponse, error) {
	u, err := s.users.FindByUsername(ctx, in.Username)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, ErrInvalidCredentials
	}
	if err != nil {
		return nil, err
	}

	if u.Status != domain.StatusActive {
		return nil, ErrUserInactive
	}

	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(in.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	token, expiresAt, err := s.signToken(u)
	if err != nil {
		return nil, err
	}

	return &TokenResponse{
		Token:     token,
		UserID:    u.ID,
		Username:  u.Username,
		Role:      u.Role,
		ExpiresAt: expiresAt,
	}, nil
}

func (s *AuthService) signToken(u *domain.User) (string, time.Time, error) {
	now := time.Now()
	expiresAt := now.Add(time.Duration(s.jwtExpiryHours) * time.Hour)
	claims := jwt.MapClaims{
		"sub":  u.ID,
		"usr":  u.Username,
		"role": u.Role,
		"iat":  now.Unix(),
		"exp":  expiresAt.Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(s.jwtSecret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, expiresAt, nil
}
