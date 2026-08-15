package groups

import "errors"

// Sentinel errors returned by gameplay persistence methods. They move here
// from the parent repository package together with the gameplay slice so the
// repository layer and the transport layer share one error vocabulary.
var (
	ErrNotFound          = errors.New("not found")
	ErrForbidden         = errors.New("forbidden")
	ErrChallengeExpired  = errors.New("challenge expired")
	ErrViewNotFinished   = errors.New("viewing window is still open")
	ErrOwnPhoto          = errors.New("cannot use own challenge")
	ErrAlreadyGuessed    = errors.New("guess already submitted")
	ErrInvalidCoordinate = errors.New("invalid coordinate")

	// ErrNotMember is the canonical membership failure returned by
	// RequireMember. Every gameplay handler maps it to 403 forbidden; one
	// sentinel keeps the membership rule centralized.
	ErrNotMember = errors.New("not a group member")

	// ErrTooManyGroupInvites is returned when a group already holds its
	// maximum number of active invites.
	ErrTooManyGroupInvites = errors.New("too many active group invites")

	// ErrTooManyUserInvites is returned when a user has already created the
	// maximum number of invites today.
	ErrTooManyUserInvites = errors.New("too many invites created today")
)
