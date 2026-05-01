package handler

import (
	"errors"
	"net/http"

	"github.com/xuefz/world-fog/internal/middleware"
	"github.com/xuefz/world-fog/internal/store"
)

type MeHandler struct {
	users *store.UserStore
}

func NewMeHandler(users *store.UserStore) *MeHandler {
	return &MeHandler{users: users}
}

// GET /api/v1/me
func (h *MeHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := h.users.GetByID(r.Context(), claims.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch user")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"user_id":          user.ID,
		"display_name":     user.DisplayName,
		"credential_count": len(user.Credentials),
	})
}
