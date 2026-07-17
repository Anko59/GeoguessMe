package auth

import (
	"context"
	"geoguessme/internal/repository"
)

// IsGroupMember checks if a user is a member of a group
func IsGroupMember(ctx context.Context, groupID, userID string) (bool, error) {
	return repository.IsGroupMemberContext(ctx, groupID, userID)
}

// VerifyGroupMembership returns an error if user is not a group member
func VerifyGroupMembership(ctx context.Context, groupID, userID string) error {
	isMember, err := IsGroupMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return &AuthorizationError{Message: "User is not a member of this group"}
	}
	return nil
}

// AuthorizationError represents an authorization failure
type AuthorizationError struct {
	Message string
}

func (e *AuthorizationError) Error() string {
	return e.Message
}
