package game

import (
	"time"

	"geoguessme/internal/models"
)

// LocationHidden applies the poster's timed location privacy setting.
func LocationHidden(photo *models.Photo, viewerID string, now time.Time, hideDuration time.Duration) bool {
	return photo.HideLocation && photo.UserID != viewerID && now.Before(photo.CreatedAt.Add(hideDuration))
}
