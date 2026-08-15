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
	chatHub "geoguessme/internal/chat"
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
	kicker chatHub.SocketKicker
	oidc   authsvc.IdentityVerifier
}

// NewAuthAPI constructs the auth transport with its explicit dependencies.
// kicker closes live WebSocket sockets after credential revocation; it is
// nil-safe (tests and unconfigured hubs rely on the hub's periodic
// revalidation instead).
func NewAuthAPI(repos *repository.Repository, cfg *config.Config, store storage.ObjectStore, mailer email.Sender, svc *authsvc.Service, kicker chatHub.SocketKicker, verifiers ...authsvc.IdentityVerifier) *AuthAPI {
	api := &AuthAPI{repos: repos, cfg: cfg, store: store, mailer: mailer, svc: svc, kicker: kicker}
	if len(verifiers) > 0 {
		api.oidc = verifiers[0]
	}
	return api
}

// kickDisconnectUser closes every live socket for a user after their
// credentials were revoked. It is best-effort and nil-safe: a missing kicker
// is ignored, and the hub's periodic revalidation remains the backstop.
func (a *AuthAPI) kickDisconnectUser(userID string) {
	if a.kicker != nil && userID != "" {
		a.kicker.DisconnectUser(userID)
	}
}
