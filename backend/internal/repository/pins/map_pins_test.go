package pins

import (
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v5"
)

func mockPinsRepository(t *testing.T) (*Repository, pgxmock.PgxPoolIface) {
	t.Helper()
	pool, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := pool.ExpectationsWereMet(); err != nil {
			t.Error(err)
		}
		pool.Close()
	})
	return NewRepository(pool), pool
}

func TestMapPinsForUserBuildsCatalogAndUnlockProgress(t *testing.T) {
	repo, pool := mockPinsRepository(t)
	unlockedAt := time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)
	pool.ExpectQuery("SELECT p.pin_key, p.name, p.description, p.image_url").WithArgs("user").WillReturnRows(
		pgxmock.NewRows([]string{"pin_key", "name", "description", "image_url", "challenge_key", "challenge_name", "challenge_description", "unlocked_at", "selected"}).
			AddRow("north-star", "North Star", "Marker description.", "/map-pins/north-star.svg", "perfect", "Perfect guess", "Get a perfect score.", unlockedAt, true).
			AddRow("north-star", "North Star", "Marker description.", "/map-pins/north-star.svg", "weekly", "Weekly leader", "Finish first this week.", nil, true),
	)
	catalog, err := repo.MapPinsForUser(t.Context(), "user")
	if err != nil {
		t.Fatalf("MapPinsForUser = %v", err)
	}
	if catalog.SelectedPinKey == nil || *catalog.SelectedPinKey != "north-star" || len(catalog.Pins) != 1 {
		t.Fatalf("catalog selection and pins = %+v", catalog)
	}
	pin := catalog.Pins[0]
	if !pin.Unlocked || len(pin.Challenges) != 2 || pin.Challenges[0].UnlockedAt == nil || pin.Challenges[1].UnlockedAt != nil {
		t.Fatalf("pin unlock progress = %+v", pin)
	}
}

func TestEquippedMapPinReturnsUnlockAttribution(t *testing.T) {
	repo, pool := mockPinsRepository(t)
	pool.ExpectQuery("SELECT p.pin_key, p.name, p.image_url, p.description").WithArgs("user").WillReturnRows(
		pgxmock.NewRows([]string{"pin_key", "name", "image_url", "description", "challenge_key", "challenge_name", "challenge_description"}).
			AddRow("north-star", "North Star", "/map-pins/north-star.svg", "Marker description.", "perfect", "Perfect guess", "Get a perfect score."),
	)
	pin, err := repo.EquippedMapPin(t.Context(), "user")
	if err != nil || pin == nil || pin.UnlockedBy.Key != "perfect" || pin.Name != "North Star" {
		t.Fatalf("EquippedMapPin = %+v, %v", pin, err)
	}
}

func TestEquipMapPinRequiresUnlockAndCanRestoreStandard(t *testing.T) {
	repo, pool := mockPinsRepository(t)
	pool.ExpectExec("INSERT INTO user_equipped_map_pins").WithArgs("user", "locked").
		WillReturnResult(pgxmock.NewResult("INSERT", 0))
	if err := repo.EquipMapPin(t.Context(), "user", "locked"); !errors.Is(err, ErrMapPinUnavailable) {
		t.Fatalf("EquipMapPin locked error = %v", err)
	}
	pool.ExpectExec("INSERT INTO user_equipped_map_pins").WithArgs("user", "unlocked").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	if err := repo.EquipMapPin(t.Context(), "user", "unlocked"); err != nil {
		t.Fatalf("EquipMapPin unlocked error = %v", err)
	}
	pool.ExpectExec("DELETE FROM user_equipped_map_pins").WithArgs("user").
		WillReturnResult(pgxmock.NewResult("DELETE", 1))
	if err := repo.ClearEquippedMapPin(t.Context(), "user"); err != nil {
		t.Fatalf("ClearEquippedMapPin = %v", err)
	}
}

func TestAwardMapPinUnlockIsIdempotentAndRejectsUnavailableChallenge(t *testing.T) {
	repo, pool := mockPinsRepository(t)
	now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	pool.ExpectExec("INSERT INTO user_map_pin_unlocks").WithArgs("user", "perfect", now).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))
	created, err := repo.AwardMapPinUnlock(t.Context(), "user", "perfect", now)
	if err != nil || !created {
		t.Fatalf("first award = %t, %v", created, err)
	}
	pool.ExpectExec("INSERT INTO user_map_pin_unlocks").WithArgs("user", "perfect", now).
		WillReturnResult(pgxmock.NewResult("INSERT", 0))
	pool.ExpectQuery("SELECT EXISTS").WithArgs("user", "perfect").WillReturnRows(
		pgxmock.NewRows([]string{"exists"}).AddRow(true),
	)
	created, err = repo.AwardMapPinUnlock(t.Context(), "user", "perfect", now)
	if err != nil || created {
		t.Fatalf("duplicate award = %t, %v", created, err)
	}
	pool.ExpectExec("INSERT INTO user_map_pin_unlocks").WithArgs("user", "missing", now).
		WillReturnResult(pgxmock.NewResult("INSERT", 0))
	pool.ExpectQuery("SELECT EXISTS").WithArgs("user", "missing").WillReturnRows(
		pgxmock.NewRows([]string{"exists"}).AddRow(false),
	)
	if _, err := repo.AwardMapPinUnlock(t.Context(), "user", "missing", now); !errors.Is(err, ErrMapPinUnavailable) {
		t.Fatalf("unavailable award error = %v", err)
	}
}
