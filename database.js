'use strict';

const Database = require('better-sqlite3');
const path = require('path');

const DB_PATH = process.env.DB_PATH || './worldfog.db';

const db = new Database(path.resolve(DB_PATH));

// 开启 WAL 模式提高并发性能
db.pragma('journal_mode = WAL');

// 建表
db.exec(`
  CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    display_name TEXT NOT NULL,
    created_at INTEGER NOT NULL DEFAULT (unixepoch())
  );

  CREATE TABLE IF NOT EXISTS credentials (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    public_key BLOB NOT NULL,
    counter INTEGER NOT NULL DEFAULT 0,
    device_type TEXT NOT NULL,
    backed_up INTEGER NOT NULL DEFAULT 0,
    transports TEXT,
    created_at INTEGER NOT NULL DEFAULT (unixepoch()),
    last_used_at INTEGER
  );

  CREATE TABLE IF NOT EXISTS challenges (
    user_id TEXT NOT NULL,
    challenge TEXT NOT NULL,
    type TEXT NOT NULL,
    expires_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, type)
  );
`);

// 自动清理过期 challenge
const cleanupStmt = db.prepare('DELETE FROM challenges WHERE expires_at < unixepoch()');

module.exports = {
  // ── Users ──────────────────────────────────────────────
  createUser(id, username, displayName) {
    return db
      .prepare('INSERT INTO users (id, username, display_name) VALUES (?, ?, ?)')
      .run(id, username, displayName);
  },

  getUserByUsername(username) {
    return db.prepare('SELECT * FROM users WHERE username = ?').get(username);
  },

  getUserById(id) {
    return db.prepare('SELECT * FROM users WHERE id = ?').get(id);
  },

  // ── Credentials ────────────────────────────────────────
  saveCredential(cred) {
    return db
      .prepare(`
        INSERT INTO credentials (id, user_id, public_key, counter, device_type, backed_up, transports)
        VALUES (?, ?, ?, ?, ?, ?, ?)
      `)
      .run(
        cred.id,
        cred.userId,
        cred.publicKey,
        cred.counter,
        cred.deviceType,
        cred.backedUp ? 1 : 0,
        cred.transports ? JSON.stringify(cred.transports) : null
      );
  },

  getCredentialsByUserId(userId) {
    const rows = db.prepare('SELECT * FROM credentials WHERE user_id = ?').all(userId);
    return rows.map(parseCredential);
  },

  getCredentialById(id) {
    const row = db.prepare('SELECT * FROM credentials WHERE id = ?').get(id);
    return row ? parseCredential(row) : null;
  },

  updateCredentialCounter(id, counter) {
    db.prepare('UPDATE credentials SET counter = ?, last_used_at = unixepoch() WHERE id = ?')
      .run(counter, id);
  },

  // ── Challenges ─────────────────────────────────────────
  saveChallenge(userId, challenge, type) {
    cleanupStmt.run();
    // 5 分钟有效期
    const expiresAt = Math.floor(Date.now() / 1000) + 300;
    db.prepare(`
      INSERT OR REPLACE INTO challenges (user_id, challenge, type, expires_at)
      VALUES (?, ?, ?, ?)
    `).run(userId, challenge, type, expiresAt);
  },

  consumeChallenge(userId, type) {
    cleanupStmt.run();
    const row = db
      .prepare('SELECT challenge FROM challenges WHERE user_id = ? AND type = ? AND expires_at > unixepoch()')
      .get(userId, type);
    if (row) {
      db.prepare('DELETE FROM challenges WHERE user_id = ? AND type = ?').run(userId, type);
      return row.challenge;
    }
    return null;
  },
};

function parseCredential(row) {
  return {
    ...row,
    backedUp: row.backed_up === 1,
    transports: row.transports ? JSON.parse(row.transports) : [],
  };
}
