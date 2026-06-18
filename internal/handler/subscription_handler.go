package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/tifacusil/go-user-management/internal/domain"
	appmiddleware "github.com/tifacusil/go-user-management/internal/middleware"
	"github.com/tifacusil/go-user-management/internal/repository"
	"github.com/tifacusil/go-user-management/internal/service"
)

type SubscriptionHandler struct {
	subs     *service.SubscriptionService
	validate *validator.Validate
}

func NewSubscriptionHandler(subs *service.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{
		subs:     subs,
		validate: validator.New(),
	}
}

// POST /api/v1/users/{id}/subscriptions
func (h *SubscriptionHandler) Create(w http.ResponseWriter, r *http.Request) {
	targetID, ok := parseUserID(w, r)
	if !ok {
		return
	}
	if !canAccessUser(r, targetID) {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	var in service.CreateSubscriptionInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.validate.Struct(in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sub, err := h.subs.Create(r.Context(), targetID, in)
	if err != nil {
		if errors.Is(err, service.ErrProjectNotFound) {
			writeError(w, http.StatusUnprocessableEntity, "project not found")
			return
		}
		if errors.Is(err, service.ErrServiceUnavailable) {
			writeError(w, http.StatusServiceUnavailable, "project service temporarily unavailable")
			return
		}
		if errors.Is(err, repository.ErrConflict) {
			writeError(w, http.StatusConflict, "subscription for this project already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to create subscription")
		return
	}

	writeJSON(w, http.StatusCreated, sub)
}

// GET /api/v1/users/{id}/subscriptions
func (h *SubscriptionHandler) List(w http.ResponseWriter, r *http.Request) {
	targetID, ok := parseUserID(w, r)
	if !ok {
		return
	}
	if !canAccessUser(r, targetID) {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	subs, err := h.subs.ListByUser(r.Context(), targetID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list subscriptions")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": subs})
}

func canAccessUser(r *http.Request, targetID int64) bool {
	callerRole := appmiddleware.RoleFromContext(r.Context())
	callerID := appmiddleware.UserIDFromContext(r.Context())
	return callerRole == domain.RoleAdmin || callerID == targetID
}
