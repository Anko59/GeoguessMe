package pins

import "geoguessme/internal/database"

// Repository is bound to the database pool injected by the composition root.
type Repository struct {
	pool database.Pool
}

// NewRepository creates the map-pin persistence slice.
func NewRepository(pool database.Pool) *Repository { return &Repository{pool: pool} }
