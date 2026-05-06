CREATE TABLE IF NOT EXISTS users (
    id           TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    created_at   INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE IF NOT EXISTS credentials (
    id              TEXT PRIMARY KEY,
    user_id         TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    credential_json BLOB NOT NULL,
    sign_count      INTEGER NOT NULL DEFAULT 0,
    backup_eligible INTEGER NOT NULL DEFAULT 0,
    backup_state    INTEGER NOT NULL DEFAULT 0,
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    last_used_at    INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_credentials_user_id ON credentials(user_id);

CREATE TABLE IF NOT EXISTS webauthn_sessions (
    id           TEXT PRIMARY KEY,
    session_data BLOB    NOT NULL,
    expires_at   INTEGER NOT NULL,
    created_at   INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX IF NOT EXISTS idx_sessions_expires ON webauthn_sessions(expires_at);

CREATE TABLE IF NOT EXISTS fog_tiles (
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    tile_key      TEXT NOT NULL,
    version       INTEGER NOT NULL DEFAULT 1,
    blob          BLOB NOT NULL,
    checksum      TEXT NOT NULL DEFAULT '',
    updated_at_ms INTEGER NOT NULL DEFAULT (CAST(strftime('%s', 'now') AS INTEGER) * 1000 + CAST(substr(strftime('%f', 'now'), 4, 3) AS INTEGER)),
    created_at    INTEGER NOT NULL DEFAULT (unixepoch()),
    PRIMARY KEY (user_id, tile_key)
);

CREATE INDEX IF NOT EXISTS idx_fog_tiles_user_updated ON fog_tiles(user_id, updated_at_ms);
