package pins

import (
	"sync"

	"geoguessme/internal/database"
	"geoguessme/internal/geography"
)

// Repository is bound to the database pool injected by the composition root.
type Repository struct {
	pool        database.Pool
	countryOnce sync.Once
	countries   *geography.Index
	countryErr  error
}

// NewRepository creates the map-pin persistence slice.
func NewRepository(pool database.Pool) *Repository { return &Repository{pool: pool} }
