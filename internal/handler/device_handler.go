package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	appmiddleware "github.com/tifacusil/go-user-management/internal/middleware"
	"github.com/tifacusil/go-user-management/internal/observability"
	"github.com/tifacusil/go-user-management/internal/repository"
	"github.com/tifacusil/go-user-management/internal/service"
)

type DeviceHandler struct {
	devices  *service.DeviceService
	validate *validator.Validate
}

func NewDeviceHandler(devices *service.DeviceService) *DeviceHandler {
	return &DeviceHandler{
		devices:  devices,
		validate: validator.New(),
	}
}

// POST /api/v1/users/{id}/devices
func (h *DeviceHandler) Register(w http.ResponseWriter, r *http.Request) {
	targetID, ok := parseUserID(w, r)
	if !ok {
		return
	}
	if !canAccessUser(r, targetID) {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	var in service.RegisterDeviceInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.validate.Struct(in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	dev, err := h.devices.Register(r.Context(), targetID, in)
	if err != nil {
		observability.FromContext(r.Context()).Error("device registration failed",
			slog.String("event", "device_register_failed"),
			slog.Int64("user_id", targetID),
			slog.String("platform", in.Platform),
			slog.String("err", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to register device")
		return
	}

	observability.FromContext(r.Context()).Info("device registered",
		slog.String("event", "device_registered"),
		slog.Int64("user_id", targetID),
		slog.Int64("device_id", dev.ID),
		slog.String("platform", dev.Platform))
	writeJSON(w, http.StatusOK, dev)
}

// GET /api/v1/users/{id}/devices
func (h *DeviceHandler) List(w http.ResponseWriter, r *http.Request) {
	targetID, ok := parseUserID(w, r)
	if !ok {
		return
	}
	if !canAccessUser(r, targetID) {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	devices, err := h.devices.ListActive(r.Context(), targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list devices")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

// DELETE /api/v1/users/{id}/devices/{deviceId}
func (h *DeviceHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	targetID, ok := parseUserID(w, r)
	if !ok {
		return
	}
	if !canAccessUser(r, targetID) {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	deviceID, err := strconv.ParseInt(chi.URLParam(r, "deviceId"), 10, 64)
	if err != nil || deviceID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid device id")
		return
	}

	callerID := appmiddleware.UserIDFromContext(r.Context())
	callerRole := appmiddleware.RoleFromContext(r.Context())

	log := observability.FromContext(r.Context())
	if err := h.devices.Revoke(r.Context(), deviceID, callerID, callerRole); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "device not found")
			return
		}
		log.Error("device revoke failed",
			slog.String("event", "device_revoke_failed"),
			slog.Int64("device_id", deviceID),
			slog.Int64("caller_id", callerID),
			slog.String("err", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to revoke device")
		return
	}

	log.Info("device revoked",
		slog.String("event", "device_revoked"),
		slog.Int64("device_id", deviceID),
		slog.Int64("caller_id", callerID))
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/v1/users/logout
//
// Revokes all active device tokens for the authenticated caller. The JWT itself
// stays valid until expiry — the client discards it after this returns.
func (h *DeviceHandler) Logout(w http.ResponseWriter, r *http.Request) {
	callerID := appmiddleware.UserIDFromContext(r.Context())
	log := observability.FromContext(r.Context())
	if err := h.devices.Logout(r.Context(), callerID); err != nil {
		log.Error("logout failed",
			slog.String("event", "logout_failed"),
			slog.Int64("user_id", callerID),
			slog.String("err", err.Error()))
		writeError(w, http.StatusInternalServerError, "failed to log out")
		return
	}
	log.Info("user logged out",
		slog.String("event", "logout"),
		slog.Int64("user_id", callerID))
	w.WriteHeader(http.StatusNoContent)
}

