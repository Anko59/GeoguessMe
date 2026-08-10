package groups

import (
	"errors"

	"geoguessme/internal/models"

	"github.com/jackc/pgx/v5"
)

// scanPhoto scans a photo row in the canonical column order of the photos
// table. It is the single owner of photo row scanning; every method in this
// package that reads photo rows goes through it.
func scanPhoto(row interface{ Scan(...any) error }) (*models.Photo, error) {
	var photo models.Photo
	err := row.Scan(&photo.ID, &photo.UserID, &photo.GroupID, &photo.URL, &photo.StorageKey, &photo.MIMEType, &photo.ByteSize, &photo.Lat, &photo.Long, &photo.LifecycleStatus, &photo.HideLocation, &photo.CreatedAt, &photo.ExpiresAt, &photo.RetentionAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &photo, nil
}

// scanGroup scans a group row in the canonical column order of the groups
// table.
func scanGroup(row interface{ Scan(...any) error }) (*models.Group, error) {
	var group models.Group
	err := row.Scan(&group.ID, &group.Name, &group.Code, &group.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// scanGroupInvite scans a group_invites row in the canonical column order of
// the group_invites table. It is the single owner of invite row scanning.
func scanGroupInvite(row interface{ Scan(...any) error }) (*models.GroupInvite, error) {
	var invite models.GroupInvite
	err := row.Scan(&invite.ID, &invite.GroupID, &invite.CreatorUserID, &invite.TokenHash, &invite.CreatedAt, &invite.ExpiresAt, &invite.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &invite, nil
}

// scanGroupPhoto scans a group_photos row in the canonical column order of the
// group_photos table.
func scanGroupPhoto(row interface{ Scan(...any) error }) (*models.GroupPhoto, error) {
	var photo models.GroupPhoto
	err := row.Scan(&photo.GroupID, &photo.StorageKey, &photo.MIMEType, &photo.ByteSize, &photo.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &photo, nil
}
