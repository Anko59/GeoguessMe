package handlers

import (
	"geoguessme/internal/repository"
	"net/http"
)

func GetUserGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}

	userID := GetUserIDFromContext(r)

	groups, err := repository.GetUserGroups(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "Unable to load groups")
		return
	}

	writeJSON(w, http.StatusOK, groups)
}
