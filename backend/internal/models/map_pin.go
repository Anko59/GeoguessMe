package models

import "time"

// MapPin is the public, image-backed identity used for a player's map marker.
type MapPin struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}

// MapPinUnlockChallenge is the challenge credited for unlocking an equipped pin.
type MapPinUnlockChallenge struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ProfileMapPin adds the persisted unlock reason to a pin shown on profiles.
type ProfileMapPin struct {
	MapPin
	Description string                `json:"description"`
	UnlockedBy  MapPinUnlockChallenge `json:"unlocked_by"`
}

// MapPinChallengeProgress reports one unlock route in the authenticated pin catalog.
type MapPinChallengeProgress struct {
	Key         string     `json:"key"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	UnlockedAt  *time.Time `json:"unlocked_at,omitempty"`
}

// MapPinChoice is one catalog entry and the unlock routes visible to its owner.
type MapPinChoice struct {
	MapPin
	Description string                    `json:"description"`
	Unlocked    bool                      `json:"unlocked"`
	Challenges  []MapPinChallengeProgress `json:"challenges"`
}

// MapPinCatalog is the authenticated player's selection and available pin catalog.
type MapPinCatalog struct {
	SelectedPinKey *string        `json:"selected_pin_key"`
	Pins           []MapPinChoice `json:"pins"`
}
