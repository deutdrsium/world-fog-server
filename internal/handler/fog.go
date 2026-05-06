package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"regexp"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/xuefz/world-fog/internal/middleware"
	"github.com/xuefz/world-fog/internal/store"
)

var fogTileKeyPattern = regexp.MustCompile(`^[0-9]+/[+-]?[0-9]+/[+-]?[0-9]+$`)

type FogHandler struct {
	tiles *store.FogTileStore
}

func NewFogHandler(tiles *store.FogTileStore) *FogHandler {
	return &FogHandler{tiles: tiles}
}

// GET /api/v1/fog/tiles?since=0&limit=500
func (h *FogHandler) ListTiles(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	sinceMs, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	tiles, err := h.tiles.List(r.Context(), claims.UserID, sinceMs, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list fog tiles")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tiles": tiles})
}

// GET /api/v1/fog/tiles/{z}/{x}/{y}
func (h *FogHandler) GetTile(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tileKey := tileKeyFromRoute(r)
	if !isValidFogTileKey(tileKey) {
		writeError(w, http.StatusBadRequest, "invalid tile key")
		return
	}

	tile, err := h.tiles.Get(r.Context(), claims.UserID, tileKey)
	if err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "tile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to get fog tile")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("ETag", strconv.FormatInt(tile.Version, 10))
	w.Header().Set("X-Fog-Tile-Version", strconv.FormatInt(tile.Version, 10))
	w.Header().Set("X-Fog-Tile-Checksum", tile.Checksum)
	w.Header().Set("X-Fog-Tile-Updated-At", strconv.FormatInt(tile.UpdatedAtMs, 10))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(tile.Blob)
}

// PUT /api/v1/fog/tiles/{z}/{x}/{y}
func (h *FogHandler) PutTile(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	tileKey := tileKeyFromRoute(r)
	if !isValidFogTileKey(tileKey) {
		writeError(w, http.StatusBadRequest, "invalid tile key")
		return
	}

	expectedVersion, err := strconv.ParseInt(r.Header.Get("X-Fog-Tile-Version"), 10, 64)
	if err != nil || expectedVersion < 0 {
		writeError(w, http.StatusBadRequest, "X-Fog-Tile-Version required")
		return
	}

	body := http.MaxBytesReader(w, r.Body, store.MaxFogTileBlobBytes+1)
	blob, err := io.ReadAll(body)
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "tile blob too large")
		return
	}
	if len(blob) == 0 {
		writeError(w, http.StatusBadRequest, "tile blob required")
		return
	}
	if len(blob) > store.MaxFogTileBlobBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "tile blob too large")
		return
	}

	checksum := r.Header.Get("X-Fog-Tile-Checksum")
	if checksum == "" {
		sum := sha256.Sum256(blob)
		checksum = hex.EncodeToString(sum[:])
	}

	meta, ok, err := h.tiles.Upsert(r.Context(), claims.UserID, tileKey, expectedVersion, blob, checksum)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save fog tile")
		return
	}
	if !ok {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":           "tile version conflict",
			"current_version": meta.Version,
		})
		return
	}

	writeJSON(w, http.StatusOK, meta)
}

func tileKeyFromRoute(r *http.Request) string {
	return chi.URLParam(r, "z") + "/" + chi.URLParam(r, "x") + "/" + chi.URLParam(r, "y")
}

func isValidFogTileKey(tileKey string) bool {
	return fogTileKeyPattern.MatchString(tileKey) && len(tileKey) <= 64
}
