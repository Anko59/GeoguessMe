// Package auth serves the authentication, profile, and avatar slices from
// injected dependencies (PR 7). The AuthAPI owns transport only: request
// parsing, token/session behavior, authorization delegation, and response
// writing. Persistence lives in internal/repository; the object store, mail
// sender, configuration, and the explicit auth token service are injected.
// The package replaced the package-level handlers that read the RuntimeConfig,
// MediaStore, and Mailer globals, and replaced the package-level auth token
// state (internal/auth.Init*) with the explicit auth.Service.
package auth

import (
	authsvc "geoguessme/internal/auth"
	"geoguessme/internal/config"
	"geoguessme/internal/email"
	"geoguessme/internal/repository"
	"geoguessme/internal/storage"
)

// AuthAPI serves the auth, profile, and avatar handlers from explicit
// dependencies. Instances are independent: two Apps construct separate
// AuthAPI values backed by separate repositories, stores, mailers, and token
// services, so no handler state is shared.
type AuthAPI struct {
	repos  *repository.Repository
	cfg    *config.Config
	store  storage.ObjectStore
	mailer email.Sender
	svc    *authsvc.Service
}

// NewAuthAPI constructs the auth transport with its explicit dependencies.
func NewAuthAPI(repos *repository.Repository, cfg *config.Config, store storage.ObjectStore, mailer email.Sender, svc *authsvc.Service) *AuthAPI {
	return &AuthAPI{repos: repos, cfg: cfg, store: store, mailer: mailer, svc: svc}
}
