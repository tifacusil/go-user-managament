package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/tifacusil/go-user-management/internal/observability"
	"github.com/tifacusil/go-user-management/internal/repository"
	"github.com/tifacusil/go-user-management/internal/service"
)

type AuthHandler struct {
	auth     *service.AuthService
	validate *validator.Validate
}

func NewAuthHandler(auth *service.AuthService) *AuthHandler {
	return &AuthHandler{
		auth:     auth,
		validate: validator.New(),
	}
}

// POST /api/v1/users/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var in service.RegisterInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.validate.Struct(in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	log := observability.FromContext(r.Context())

	u, err := h.auth.Register(r.Context(), in)
	if err != nil {
		if errors.Is(err, repository.ErrConflict) {
			log.Warn("registration rejected: conflict",
				slog.String("event", "user_register_failed"),
				slog.String("username", in.Username),
				slog.String("reason", "username_or_email_taken"))
			writeError(w, http.StatusConflict, "username or email already taken")
			return
		}
		log.Error("registration failed",
			slog.String("event", "user_register_failed"),
			slog.String("username", in.Username),
			slog.String("err", err.Error()))
		writeError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	log.Info("user registered",
		slog.String("event", "user_registered"),
		slog.Int64("user_id", u.ID),
		slog.String("username", u.Username),
		slog.String("role", u.Role))
	writeJSON(w, http.StatusCreated, u)
}

// POST /api/v1/users/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var in service.LoginInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.validate.Struct(in); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	log := observability.FromContext(r.Context())

	resp, err := h.auth.Login(r.Context(), in)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			log.Warn("login failed: invalid credentials",
				slog.String("event", "login_failed"),
				slog.String("username", in.Username),
				slog.String("reason", "invalid_credentials"))
			writeError(w, http.StatusUnauthorized, "invalid username or password")
			return
		}
		if errors.Is(err, service.ErrUserInactive) {
			log.Warn("login failed: account inactive",
				slog.String("event", "login_failed"),
				slog.String("username", in.Username),
				slog.String("reason", "account_inactive"))
			writeError(w, http.StatusForbidden, "account is not active")
			return
		}
		log.Error("login failed",
			slog.String("event", "login_failed"),
			slog.String("username", in.Username),
			slog.String("err", err.Error()))
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	log.Info("login succeeded",
		slog.String("event", "login"),
		slog.Int64("user_id", resp.UserID),
		slog.String("username", resp.Username),
		slog.String("role", resp.Role))
	writeJSON(w, http.StatusOK, resp)
}
