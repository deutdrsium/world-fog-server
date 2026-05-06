package store

import (
	"context"
	"database/sql"
	"errors"
)

const MaxFogTileBlobBytes = 256 * 1024

type FogTile struct {
	TileKey     string
	Version     int64
	Blob        []byte
	Checksum    string
	UpdatedAtMs int64
}

type FogTileMeta struct {
	TileKey     string `json:"tile_key"`
	Version     int64  `json:"version"`
	Checksum    string `json:"checksum,omitempty"`
	SizeBytes   int    `json:"size_bytes"`
	UpdatedAtMs int64  `json:"updated_at_ms"`
}

type FogTileStore struct {
	db *sql.DB
}

func NewFogTileStore(db *sql.DB) *FogTileStore {
	return &FogTileStore{db: db}
}

func (s *FogTileStore) List(ctx context.Context, userID string, sinceMs int64, limit int) ([]FogTileMeta, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT tile_key, version, checksum, length(blob), updated_at_ms
		FROM fog_tiles
		WHERE user_id = ? AND updated_at_ms > ?
		ORDER BY updated_at_ms ASC, tile_key ASC
		LIMIT ?
	`, userID, sinceMs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tiles := make([]FogTileMeta, 0, limit)
	for rows.Next() {
		var tile FogTileMeta
		if err := rows.Scan(&tile.TileKey, &tile.Version, &tile.Checksum, &tile.SizeBytes, &tile.UpdatedAtMs); err != nil {
			return nil, err
		}
		tiles = append(tiles, tile)
	}
	return tiles, rows.Err()
}

func (s *FogTileStore) Get(ctx context.Context, userID, tileKey string) (*FogTile, error) {
	var tile FogTile
	err := s.db.QueryRowContext(ctx, `
		SELECT tile_key, version, blob, checksum, updated_at_ms
		FROM fog_tiles
		WHERE user_id = ? AND tile_key = ?
	`, userID, tileKey).Scan(&tile.TileKey, &tile.Version, &tile.Blob, &tile.Checksum, &tile.UpdatedAtMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &tile, nil
}

func (s *FogTileStore) Upsert(ctx context.Context, userID, tileKey string, expectedVersion int64, blob []byte, checksum string) (FogTileMeta, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return FogTileMeta{}, false, err
	}
	defer tx.Rollback()

	var currentVersion int64
	err = tx.QueryRowContext(ctx, `
		SELECT version
		FROM fog_tiles
		WHERE user_id = ? AND tile_key = ?
	`, userID, tileKey).Scan(&currentVersion)
	if errors.Is(err, sql.ErrNoRows) {
		if expectedVersion != 0 {
			return FogTileMeta{}, false, nil
		}
	} else if err != nil {
		return FogTileMeta{}, false, err
	} else if currentVersion != expectedVersion {
		return FogTileMeta{
			TileKey: tileKey,
			Version: currentVersion,
		}, false, nil
	}

	nextVersion := currentVersion + 1
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO fog_tiles (user_id, tile_key, version, blob, checksum, updated_at_ms)
		VALUES (?, ?, ?, ?, ?, CAST(strftime('%s', 'now') AS INTEGER) * 1000 + CAST(substr(strftime('%f', 'now'), 4, 3) AS INTEGER))
		ON CONFLICT(user_id, tile_key) DO UPDATE SET
			version = excluded.version,
			blob = excluded.blob,
			checksum = excluded.checksum,
			updated_at_ms = excluded.updated_at_ms
	`, userID, tileKey, nextVersion, blob, checksum); err != nil {
		return FogTileMeta{}, false, err
	}

	var meta FogTileMeta
	if err := tx.QueryRowContext(ctx, `
		SELECT tile_key, version, checksum, length(blob), updated_at_ms
		FROM fog_tiles
		WHERE user_id = ? AND tile_key = ?
	`, userID, tileKey).Scan(&meta.TileKey, &meta.Version, &meta.Checksum, &meta.SizeBytes, &meta.UpdatedAtMs); err != nil {
		return FogTileMeta{}, false, err
	}

	if err := tx.Commit(); err != nil {
		return FogTileMeta{}, false, err
	}
	return meta, true, nil
}
