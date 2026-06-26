package service

import (
	"context"
	"errors"
	"time"

	"github.com/tifacusil/go-user-management/internal/domain"
	"github.com/tifacusil/go-user-management/internal/repository"
)

type DeviceService struct {
	devices *repository.DeviceRepository
}

func NewDeviceService(devices *repository.DeviceRepository) *DeviceService {
	return &DeviceService{devices: devices}
}

type RegisterDeviceInput struct {
	DeviceToken string `json:"device_token" validate:"required"`
	Platform    string `json:"platform"     validate:"required,oneof=android ios web"`
	AppVersion  string `json:"app_version"  validate:"omitempty"`
}

// Register adds or transfers a device token to userID.
// If the token is already active on a different user, it is silently reassigned
// (the previous owner's app will fail FCM delivery and re-register naturally).
func (s *DeviceService) Register(ctx context.Context, userID int64, in RegisterDeviceInput) (*domain.UserDevice, error) {
	d := &domain.UserDevice{
		UserID:      userID,
		DeviceToken: in.DeviceToken,
		Platform:    in.Platform,
		AppVersion:  in.AppVersion,
		LastSeenAt:  time.Now().UTC(),
		Status:      domain.StatusActive,
	}

	if err := s.devices.Upsert(ctx, d); err != nil {
		return nil, err
	}

	// Reload to get the id and db-set last_seen_at if this was an update.
	existing, err := s.devices.FindByToken(ctx, in.DeviceToken)
	if err == nil {
		return existing, nil
	}
	return d, nil
}

func (s *DeviceService) ListActive(ctx context.Context, userID int64) ([]domain.UserDevice, error) {
	return s.devices.ListActiveByUserID(ctx, userID)
}

func (s *DeviceService) Revoke(ctx context.Context, deviceID, callerID int64, callerRole string) error {
	if callerRole != domain.RoleAdmin {
		// Non-admins can only revoke their own devices; Revoke checks user_id ownership.
		if err := s.devices.Revoke(ctx, deviceID, callerID); err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				return repository.ErrNotFound
			}
			return err
		}
		return nil
	}

	// Admins: load first to confirm device exists, then revoke regardless of owner.
	dev, err := s.devices.FindByID(ctx, deviceID)
	if err != nil {
		return err
	}
	return s.devices.Revoke(ctx, deviceID, dev.UserID)
}

// Logout revokes all active device tokens for the caller. The JWT stays valid
// until expiry — client discards it. Mirrors the Java AuthService.logout.
func (s *DeviceService) Logout(ctx context.Context, userID int64) error {
	return s.devices.RevokeAllByUserID(ctx, userID)
}
