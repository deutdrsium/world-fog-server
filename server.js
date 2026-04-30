'use strict';

require('dotenv').config();

const express = require('express');
const cors = require('cors');
const crypto = require('crypto');
const jwt = require('jsonwebtoken');
const {
  generateRegistrationOptions,
  verifyRegistrationResponse,
  generateAuthenticationOptions,
  verifyAuthenticationResponse,
} = require('@simplewebauthn/server');

const db = require('./database');

const app = express();
app.use(express.json());
app.use(cors({
  origin: process.env.RP_ORIGIN || 'https://yourdomain.com',
  credentials: true,
}));

const RP_ID     = process.env.RP_ID     || 'yourdomain.com';
const RP_NAME   = process.env.RP_NAME   || 'World Fog';
const RP_ORIGIN = process.env.RP_ORIGIN || 'https://yourdomain.com';
const JWT_SECRET = process.env.JWT_SECRET || 'change-me';
const PORT = parseInt(process.env.PORT || '3000', 10);

// ── 工具函数 ────────────────────────────────────────────────────────────────

function makeToken(userId) {
  return jwt.sign({ sub: userId }, JWT_SECRET, { expiresIn: '30d' });
}

function authMiddleware(req, res, next) {
  const header = req.headers.authorization || '';
  const token = header.startsWith('Bearer ') ? header.slice(7) : null;
  if (!token) return res.status(401).json({ error: 'Unauthorized' });
  try {
    const payload = jwt.verify(token, JWT_SECRET);
    req.userId = payload.sub;
    next();
  } catch {
    res.status(401).json({ error: 'Invalid token' });
  }
}

// ── Apple App Site Association（Passkey 关联域名必需）────────────────────────

app.get('/.well-known/apple-app-site-association', (req, res) => {
  // appID 格式：<TeamID>.<BundleID>
  // 请将下面的值替换为你的 Apple Team ID 和 App Bundle ID
  const TEAM_ID   = process.env.APPLE_TEAM_ID   || 'XXXXXXXXXX';
  const BUNDLE_ID = process.env.APPLE_BUNDLE_ID || 'com.example.WorldFog';

  res.json({
    webcredentials: {
      apps: [`${TEAM_ID}.${BUNDLE_ID}`],
    },
  });
});

// ── 注册 ────────────────────────────────────────────────────────────────────

// 1. 开始注册：返回挑战值和选项
app.post('/auth/register/begin', async (req, res) => {
  const { username, displayName } = req.body;
  if (!username || typeof username !== 'string') {
    return res.status(400).json({ error: 'username is required' });
  }

  let user = db.getUserByUsername(username);
  if (!user) {
    const id = crypto.randomUUID();
    db.createUser(id, username, displayName || username);
    user = db.getUserById(id);
  }

  const existingCredentials = db.getCredentialsByUserId(user.id);

  const options = await generateRegistrationOptions({
    rpName: RP_NAME,
    rpID: RP_ID,
    userID: user.id,
    userName: user.username,
    userDisplayName: user.display_name,
    attestationType: 'none',
    authenticatorSelection: {
      residentKey: 'required',
      userVerification: 'required',
      authenticatorAttachment: 'platform',
    },
    excludeCredentials: existingCredentials.map(c => ({
      id: c.id,
      type: 'public-key',
      transports: c.transports,
    })),
  });

  db.saveChallenge(user.id, options.challenge, 'registration');

  res.json({ userId: user.id, options });
});

// 2. 完成注册：验证凭据并保存
app.post('/auth/register/complete', async (req, res) => {
  const { userId, credential } = req.body;
  if (!userId || !credential) {
    return res.status(400).json({ error: 'userId and credential are required' });
  }

  const user = db.getUserById(userId);
  if (!user) return res.status(404).json({ error: 'User not found' });

  const expectedChallenge = db.consumeChallenge(userId, 'registration');
  if (!expectedChallenge) return res.status(400).json({ error: 'Challenge expired or not found' });

  let verification;
  try {
    verification = await verifyRegistrationResponse({
      response: credential,
      expectedChallenge,
      expectedOrigin: RP_ORIGIN,
      expectedRPID: RP_ID,
    });
  } catch (err) {
    return res.status(400).json({ error: err.message });
  }

  if (!verification.verified || !verification.registrationInfo) {
    return res.status(400).json({ error: 'Verification failed' });
  }

  const { credentialID, credentialPublicKey, counter, credentialDeviceType, credentialBackedUp } =
    verification.registrationInfo;

  db.saveCredential({
    id: Buffer.from(credentialID).toString('base64url'),
    userId: user.id,
    publicKey: Buffer.from(credentialPublicKey),
    counter,
    deviceType: credentialDeviceType,
    backedUp: credentialBackedUp,
    transports: credential.response?.transports ?? [],
  });

  const token = makeToken(user.id);
  res.json({ verified: true, token, user: { id: user.id, username: user.username } });
});

// ── 登录 ────────────────────────────────────────────────────────────────────

// 3. 开始登录：返回挑战值
app.post('/auth/login/begin', async (req, res) => {
  const { username } = req.body;

  let allowCredentials = [];
  let userId;

  if (username) {
    const user = db.getUserByUsername(username);
    if (!user) return res.status(404).json({ error: 'User not found' });
    userId = user.id;
    const creds = db.getCredentialsByUserId(user.id);
    allowCredentials = creds.map(c => ({
      id: c.id,
      type: 'public-key',
      transports: c.transports,
    }));
  } else {
    // Passkey 免用户名登录（discoverable credential）
    userId = '__anonymous__';
  }

  const options = await generateAuthenticationOptions({
    rpID: RP_ID,
    userVerification: 'required',
    allowCredentials,
  });

  db.saveChallenge(userId, options.challenge, 'authentication');

  res.json({ userId, options });
});

// 4. 完成登录：验证断言
app.post('/auth/login/complete', async (req, res) => {
  const { userId, credential } = req.body;
  if (!userId || !credential) {
    return res.status(400).json({ error: 'userId and credential are required' });
  }

  const expectedChallenge = db.consumeChallenge(userId, 'authentication');
  if (!expectedChallenge) return res.status(400).json({ error: 'Challenge expired or not found' });

  // 免用户名登录：从 credential.response.userHandle 还原用户
  let resolvedUserId = userId;
  if (userId === '__anonymous__') {
    const userHandle = credential.response?.userHandle;
    if (!userHandle) return res.status(400).json({ error: 'userHandle missing' });
    resolvedUserId = Buffer.from(userHandle, 'base64url').toString('utf8');
  }

  const user = db.getUserById(resolvedUserId);
  if (!user) return res.status(404).json({ error: 'User not found' });

  const credRecord = db.getCredentialById(credential.id);
  if (!credRecord || credRecord.user_id !== user.id) {
    return res.status(400).json({ error: 'Credential not found' });
  }

  let verification;
  try {
    verification = await verifyAuthenticationResponse({
      response: credential,
      expectedChallenge,
      expectedOrigin: RP_ORIGIN,
      expectedRPID: RP_ID,
      authenticator: {
        credentialID: new Uint8Array(Buffer.from(credRecord.id, 'base64url')),
        credentialPublicKey: new Uint8Array(credRecord.public_key),
        counter: credRecord.counter,
        transports: credRecord.transports,
      },
    });
  } catch (err) {
    return res.status(400).json({ error: err.message });
  }

  if (!verification.verified) {
    return res.status(400).json({ error: 'Verification failed' });
  }

  db.updateCredentialCounter(credential.id, verification.authenticationInfo.newCounter);

  const token = makeToken(user.id);
  res.json({ verified: true, token, user: { id: user.id, username: user.username } });
});

// ── 用户信息（示例受保护接口）─────────────────────────────────────────────

app.get('/auth/me', authMiddleware, (req, res) => {
  const user = db.getUserById(req.userId);
  if (!user) return res.status(404).json({ error: 'User not found' });
  res.json({ id: user.id, username: user.username, displayName: user.display_name });
});

// ── 启动 ─────────────────────────────────────────────────────────────────────

app.listen(PORT, () => {
  console.log(`World Fog auth server running on port ${PORT}`);
  console.log(`RP_ID: ${RP_ID}  |  ORIGIN: ${RP_ORIGIN}`);
});
