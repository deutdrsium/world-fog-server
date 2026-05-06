# 世界迷雾 服务端

iOS 应用"世界迷雾"的后端服务，提供基于 **Passkey（WebAuthn/FIDO2）** 的无密码认证。

## 部署信息

| 项目 | 值 |
|------|-----|
| API 地址 | `https://api.xuefz.cn`（端口 8443） |
| WebAuthn RPID | `xuefz.cn` |
| 协议 | HTTPS（必须，WebAuthn 要求） |

> **注意**：主域名 `xuefz.cn` 已被占用，API 服务部署在子域名 `api.xuefz.cn:8443`。  
> 但由于 iOS Passkey 要求 RPID 与 Associated Domains 一致，`/.well-known/apple-app-site-association`  
> **必须**从 `https://xuefz.cn/.well-known/apple-app-site-association` 可访问（见下文 nginx 配置）。

---

## 技术栈

- **语言**：Go 1.22+
- **HTTP 框架**：[chi](https://github.com/go-chi/chi)
- **WebAuthn 库**：[go-webauthn/webauthn](https://github.com/go-webauthn/webauthn)
- **数据库**：SQLite（[modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite)，纯 Go，无需 CGO）
- **认证**：JWT HS256（[golang-jwt/jwt](https://github.com/golang-jwt/jwt)）

---

## 快速开始

### 前置要求

- Go 1.22+
- TLS 证书（本地开发可用 [mkcert](https://github.com/FiloSottile/mkcert)）

### 1. 克隆并配置

```bash
git clone https://github.com/xuefz/world-fog
cd world-fog

cp .env.example .env
# 编辑 .env，填写必要的环境变量（见下表）
```

### 2. 本地开发运行（HTTP 模式）

```bash
# 不配置 TLS，服务器以 HTTP 模式启动（仅开发用）
WF_JWT_SECRET=dev-secret-32-bytes-change-me \
WF_APPLE_TEAM_ID=ABCDE12345 \
WF_APPLE_BUNDLE_ID=cn.xuefz.worldfog \
go run ./cmd/server --config configs/config.yaml
```

### 3. 生产运行（HTTPS 模式）

```bash
# 构建
make build

# 交叉编译 Debian amd64 版本
make build-debian

# 运行（TLS 由 Go 层处理）
WF_JWT_SECRET=<32字节随机字符串> \
WF_SERVER_TLS_CERT=/etc/ssl/certs/api.xuefz.cn.pem \
WF_SERVER_TLS_KEY=/etc/ssl/private/api.xuefz.cn-key.pem \
WF_APPLE_TEAM_ID=<你的 Apple Team ID> \
WF_APPLE_BUNDLE_ID=cn.xuefz.worldfog \
./bin/world-fog
```

### 4. Docker 运行

```bash
make docker

docker run -p 8443:8443 \
  -e WF_JWT_SECRET=change-me-32-bytes \
  -e WF_SERVER_TLS_CERT=/certs/cert.pem \
  -e WF_SERVER_TLS_KEY=/certs/key.pem \
  -e WF_APPLE_TEAM_ID=ABCDE12345 \
  -e WF_APPLE_BUNDLE_ID=cn.xuefz.worldfog \
  -v $(PWD)/certs:/certs \
  -v $(PWD)/data:/var/lib/world-fog \
  world-fog:latest
```

---

## 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `WF_JWT_SECRET` | — | **必填**，JWT 签名密钥（至少 32 字节随机字符串） |
| `WF_SERVER_HOST` | `0.0.0.0` | 监听地址 |
| `WF_SERVER_PORT` | `8443` | 监听端口 |
| `WF_SERVER_TLS_CERT` | — | TLS 证书路径（留空则以 HTTP 启动） |
| `WF_SERVER_TLS_KEY` | — | TLS 私钥路径 |
| `WF_WEBAUTHN_RP_ID` | `xuefz.cn` | WebAuthn Relying Party ID |
| `WF_WEBAUTHN_RP_DISPLAY_NAME` | `世界迷雾` | 在系统弹窗中显示的应用名称 |
| `WF_WEBAUTHN_RP_ORIGINS` | `https://xuefz.cn,https://api.xuefz.cn` | 允许的来源（逗号分隔） |
| `WF_JWT_EXPIRY_HRS` | `720` | JWT 有效期（小时，默认 30 天） |
| `WF_DB_PATH` | `./world-fog.db` | SQLite 数据库文件路径 |
| `WF_APPLE_TEAM_ID` | — | **必填**，Apple Developer Team ID（10 位字母数字） |
| `WF_APPLE_BUNDLE_ID` | — | **必填**，iOS 应用 Bundle ID（如 `cn.xuefz.worldfog`） |

---

## API 说明

完整规范见 [openapi.yaml](./openapi.yaml)。

### Passkey 注册流程

```
iOS App                              api.xuefz.cn
  │                                       │
  │── POST /api/v1/auth/register/begin ──>│  生成挑战
  │<─ { session_id, user_id, public_key } ─│
  │                                       │
  │  [用户 Face ID / Touch ID 授权]         │
  │                                       │
  │── POST /api/v1/auth/register/finish ─>│  验证并保存凭证
  │<─ { token, user_id } ─────────────────│
```

### Passkey 登录流程

```
iOS App                              api.xuefz.cn
  │                                       │
  │── POST /api/v1/auth/login/begin ─────>│  生成挑战（无需用户名）
  │<─ { session_id, public_key } ─────────│
  │                                       │
  │  [iOS 弹出该域所有 Passkey，用户选择]   │
  │  [用户 Face ID / Touch ID 授权]         │
  │                                       │
  │── POST /api/v1/auth/login/finish ────>│  验证断言
  │<─ { token, user_id } ─────────────────│
```

### Session ID 传递方式

API 不使用 Cookie，session_id 通过 JSON 字段传递：

**finish 请求体示例：**

```json
{
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "user_id": "...",
  "display_name": "张三",
  "credential": {
    "id": "<base64url>",
    "rawId": "<base64url>",
    "type": "public-key",
    "response": {
      "clientDataJSON": "<base64url>",
      "attestationObject": "<base64url>"
    }
  }
}
```

---

## iOS 端集成要点

### 1. Xcode Associated Domains 配置

在 Xcode 项目的 **Signing & Capabilities** → **Associated Domains** 中添加：

```
webcredentials:xuefz.cn
```

### 2. RPID 必须与 Associated Domain 一致

服务端 `RPID = xuefz.cn`，iOS 端创建请求时也必须使用相同值：

```swift
let provider = ASAuthorizationPlatformPublicKeyCredentialProvider(
    relyingPartyIdentifier: "xuefz.cn"
)
```

### 3. 注册示例（Swift 伪代码）

```swift
// 1. 从服务端获取 challenge
let beginResponse = try await apiClient.registerBegin(displayName: "张三")
let challengeBytes = Data(base64URLEncoded: beginResponse.publicKey.challenge)!

// 2. 创建注册请求
let registrationRequest = provider.createCredentialRegistrationRequest(
    clientDataHash: SHA256.hash(data: clientDataJSON)
)
// 实际使用 ASAuthorizationController 处理 challenge

// 3. 将结果发送到服务端
let finishBody: [String: Any] = [
    "session_id": beginResponse.sessionId,
    "user_id": beginResponse.userId,
    "display_name": "张三",
    "credential": [
        "id": registration.credentialID.base64URLEncodedString(),
        "rawId": registration.credentialID.base64URLEncodedString(),
        "type": "public-key",
        "response": [
            "clientDataJSON": registration.rawClientDataJSON.base64URLEncodedString(),
            "attestationObject": registration.rawAttestationObject.base64URLEncodedString()
        ]
    ]
]
let authResponse = try await apiClient.registerFinish(body: finishBody)
// 将 authResponse.token 存入 Keychain
```

### 4. JWT 存储

请将 JWT 存储在 **iOS Keychain**（`SecItemAdd`），不要存在 `UserDefaults`。

---

## 迷雾数据同步

服务端把迷雾进度作为用户私有二进制 blob 存储，不解密、不解压、不做区块级合并。客户端负责把本地 100m 点亮网格按地图 tile 分片，压缩加密后上传。

### 数据模型

- `tile_key`: `{z}/{x}/{y}`，例如 `12/3381/1552`
- `blob`: 客户端压缩加密后的二进制数据
- `version`: 单 tile 单调递增版本，用于乐观并发控制
- `checksum`: 客户端提供或服务端按 blob 计算的 SHA-256

### API

所有接口都需要 `Authorization: Bearer <JWT>`。

```http
GET /api/v1/fog/tiles?since=0&limit=500
```

返回 tile 元信息列表，用于增量同步。

```http
GET /api/v1/fog/tiles/{z}/{x}/{y}
```

返回 `application/octet-stream` blob。响应头包含：

- `X-Fog-Tile-Version`
- `X-Fog-Tile-Checksum`
- `X-Fog-Tile-Updated-At`

```http
PUT /api/v1/fog/tiles/{z}/{x}/{y}
X-Fog-Tile-Version: 0
X-Fog-Tile-Checksum: <sha256>
Content-Type: application/octet-stream

<encrypted compressed blob>
```

`X-Fog-Tile-Version` 是客户端基于的远端版本；新 tile 使用 `0`。版本不匹配返回 `409`，客户端应下载远端 blob，本地 OR 合并后再上传。

---

## nginx 配置（让 AASA 从主域名可访问）

在 `xuefz.cn` 的 nginx 配置中添加：

```nginx
server {
    server_name xuefz.cn www.xuefz.cn;

    # 将 AASA 请求转发到 Go 服务
    location /.well-known/apple-app-site-association {
        proxy_pass https://api.xuefz.cn:8443/.well-known/apple-app-site-association;
        proxy_set_header Host api.xuefz.cn;
        # 不缓存，确保配置变更即时生效
        add_header Cache-Control "no-cache, no-store";
    }

    location /.well-known/webauthn {
        proxy_pass https://api.xuefz.cn:8443/.well-known/webauthn;
        proxy_set_header Host api.xuefz.cn;
    }

    # ... 其他配置
}
```

或者，若你可以在 `xuefz.cn` 上直接提供静态文件，也可将 AASA 内容写为静态 JSON 文件部署。

---

## 项目结构

```
world-fog/
├── cmd/server/main.go                  # 入口点
├── internal/
│   ├── config/config.go                # 配置加载（YAML + 环境变量）
│   ├── db/
│   │   ├── db.go                       # SQLite 初始化与迁移
│   │   └── migrations/001_init.sql     # 数据库建表 SQL
│   ├── models/user.go                  # 用户模型（实现 webauthn.User 接口）
│   ├── store/
│   │   ├── user_store.go               # 用户数据库操作
│   │   ├── credential_store.go         # Passkey 凭证数据库操作
│   │   └── session_store.go            # WebAuthn 挑战会话管理
│   ├── handler/
│   │   ├── auth.go                     # 注册/登录四个端点
│   │   ├── me.go                       # 用户信息端点
│   │   └── well_known.go               # AASA + WebAuthn ROR
│   ├── middleware/
│   │   ├── cors.go                     # CORS 中间件
│   │   └── jwt.go                      # JWT 认证中间件
│   ├── token/jwt.go                    # JWT 签发与验证
│   └── webauthn/webauthn.go            # WebAuthn 实例构造
├── configs/config.yaml                 # 默认配置
├── .env.example                        # 环境变量示例
├── Dockerfile                          # 容器镜像
├── Makefile
├── openapi.yaml                        # OpenAPI 3.0 规范
└── README.md
```

---

## Makefile 命令

```bash
make build       # 编译二进制到 bin/world-fog
make run         # 开发模式运行（HTTP，使用默认配置）
make test        # 运行测试
make docker      # 构建 Docker 镜像
make tidy        # go mod tidy
```

---

## 生成随机 JWT Secret

```bash
# macOS / Linux
openssl rand -base64 32

# 或
head -c 32 /dev/urandom | base64
```
