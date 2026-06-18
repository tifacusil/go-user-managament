package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	appmiddleware "github.com/tifacusil/go-user-management/internal/middleware"
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
		writeError(w, http.StatusInternalServerError, "failed to register device")
		return
	}

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

	if err := h.devices.Revoke(r.Context(), deviceID, callerID, callerRole); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "device not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to revoke device")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

