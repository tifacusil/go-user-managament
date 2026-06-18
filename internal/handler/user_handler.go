package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/tifacusil/go-user-management/internal/domain"
	appmiddleware "github.com/tifacusil/go-user-management/internal/middleware"
	"github.com/tifacusil/go-user-management/internal/repository"
	"github.com/tifacusil/go-user-management/internal/service"
)

type UserHandler struct {
	users    *service.UserService
	validate *validator.Validate
}

func NewUserHandler(users *service.UserService) *UserHandler {
	return &UserHandler{
		users:    users,
		validate: validator.New(),
	}
}

// GET /api/v1/users/{id}
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	targetID, ok := parseUserID(w, r)
	if !ok {
		return
	}

	callerID := appmiddleware.UserIDFromContext(r.Context())
	callerRole := appmiddleware.RoleFromContext(r.Context())

	if callerRole != domain.RoleAdmin && callerID != targetID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	u, err := h.users.GetByID(r.Context(), targetID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	writeJSON(w, http.StatusOK, u)
}

// PUT /api/v1/users/{id}
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	targetID, ok := parseUserID(w, r)
	if !ok {
		return
	}

	callerID := appmiddleware.UserIDFromContext(r.Context())
	callerRole := appmiddleware.RoleFromContext(r.Context())

	if callerRole != domain.RoleAdmin && callerID != targetID {
		writeError(w, http.StatusForbidden, "access denied")
		return
	}

	var in service.UpdateUserInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.validate.Struct(in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	u, err := h.users.Update(r.Context(), targetID, callerID, callerRole, in)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		if errors.Is(err, repository.ErrConflict) {
			writeError(w, http.StatusConflict, "email already taken")
			return
		}
		if errors.Is(err, service.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, "current password is incorrect")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update user")
		return
	}

	writeJSON(w, http.StatusOK, u)
}

func parseUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return 0, false
	}
	return id, true
}
