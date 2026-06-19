package seed

import (
	"context"
	"errors"
	"log/slog"

	"github.com/tifacusil/go-user-management/internal/domain"
	"github.com/tifacusil/go-user-management/internal/repository"
	"github.com/tifacusil/go-user-management/internal/service"
)

type Options struct {
	AdminPassword string
	UserPassword  string
}

func Run(ctx context.Context, authSvc *service.AuthService, userRepo *repository.UserRepository, subRepo *repository.SubscriptionRepository, opts Options) {
	log := slog.Default()

	users := []service.RegisterInput{
		{Username: "admin", Email: "admin@landslide.test", PhoneE164: "+51999111111", Password: opts.AdminPassword, Role: domain.RoleAdmin},
		{Username: "supervisor", Email: "sup@landslide.test", PhoneE164: "+51999444444", Password: opts.UserPassword, Role: domain.RoleAdmin},
		{Username: "operador_shangtan", Email: "op.shangtan@landslide.test", PhoneE164: "+51999222222", Password: opts.UserPassword, Role: domain.RoleOperator},
		{Username: "operador_lingzhan", Email: "op.lingzhan@landslide.test", PhoneE164: "+51999333333", Password: opts.UserPassword, Role: domain.RoleOperator},
		{Username: "autoridad_regional", Email: "autoridad@gob.test", PhoneE164: "+51999555555", Password: opts.UserPassword, Role: domain.RoleViewer},
	}

	log.Info("seed: seeding users")
	userIDs := make(map[string]int64)
	for _, u := range users {
		created, err := authSvc.Register(ctx, u)
		if errors.Is(err, repository.ErrConflict) {
			existing, ferr := userRepo.FindByUsername(ctx, u.Username)
			if ferr == nil {
				userIDs[u.Username] = existing.ID
			}
			log.Info("seed: user exists, skipping", "user", u.Username)
			continue
		}
		if err != nil {
			log.Error("seed: failed to create user", "user", u.Username, "err", err)
			continue
		}
		userIDs[u.Username] = created.ID
		log.Info("seed: created user", "user", u.Username, "role", u.Role)
	}

	qs, qe := ptr("22:00"), ptr("06:00")

	type subSeed struct {
		username      string
		projectID     string
		minAlertLevel string
		sms           bool
		push          bool
		email         bool
		quietStart    *string
		quietEnd      *string
	}

	subscriptions := []subSeed{
		{"admin", "shangtan", domain.AlertBlue, true, true, true, nil, nil},
		{"admin", "lingzhan", domain.AlertBlue, true, true, true, nil, nil},
		{"operador_shangtan", "shangtan", domain.AlertBlue, true, true, false, qs, qe},
		{"operador_lingzhan", "lingzhan", domain.AlertBlue, true, true, false, nil, nil},
		{"supervisor", "shangtan", domain.AlertOrange, true, true, false, nil, nil},
		{"supervisor", "lingzhan", domain.AlertOrange, true, true, false, nil, nil},
		{"autoridad_regional", "shangtan", domain.AlertRed, true, false, true, nil, nil},
		{"autoridad_regional", "lingzhan", domain.AlertRed, true, false, true, nil, nil},
	}

	log.Info("seed: seeding subscriptions")
	for _, s := range subscriptions {
		uid, ok := userIDs[s.username]
		if !ok {
			log.Warn("seed: no user_id, skipping subscription", "user", s.username)
			continue
		}
		err := subRepo.Create(ctx, &domain.UserProjectAlertSubscription{
			UserID:          uid,
			ProjectID:       s.projectID,
			MinAlertLevel:   s.minAlertLevel,
			SMSEnabled:      s.sms,
			PushEnabled:     s.push,
			EmailEnabled:    s.email,
			QuietHoursStart: s.quietStart,
			QuietHoursEnd:   s.quietEnd,
		})
		if errors.Is(err, repository.ErrConflict) {
			log.Info("seed: subscription exists, skipping", "user", s.username, "project", s.projectID)
			continue
		}
		if err != nil {
			log.Error("seed: failed to create subscription", "user", s.username, "project", s.projectID, "err", err)
			continue
		}
		log.Info("seed: created subscription", "user", s.username, "project", s.projectID, "min_level", s.minAlertLevel)
	}

	log.Info("seed: done")
}

func ptr(s string) *string { return &s }
