# API reference

All endpoints are rooted at `/api/v1`. The canonical specification is
[openapi.yaml](openapi.yaml).

## Conventions

- **Auth**: Protected endpoints require `Authorization: Bearer <access_token>`.
  The refresh cookie (`refresh_token`, path `/api/v1/auth`, HttpOnly) is used
  automatically by the `/api/v1/auth/refresh` endpoint.
- **Request body**: JSON (`application/json`), except
  `POST /api/v1/photo/upload` which uses `multipart/form-data`.
- **Response body**: Always JSON (or image bytes for media endpoints).
- **Errors**: `{"error":{"code":"machine_readable","message":"human_readable"}}`
- **Timestamps**: ISO 8601 / RFC 3339 format in UTC.
- **Rate limits**: When exceeded, the response includes a `Retry-After` header
  with an integer number of seconds.
- **Cursor pagination**: Used for group messages. Response includes `items` and
  `next_cursor` (opaque base64-encoded). An empty `cursor` selects the most
  recent page (the newest messages, in chronological order, with no forward
  cursor); a non-empty `cursor` returns messages strictly after it. An empty
  `next_cursor` in the response means no more pages. The legacy `after_id`
  message id is resolved onto the cursor so reconnect callers that only know the
  last received message id keep working.
- Challenge messages in `items` may include viewer-specific `challenge_status`
  (`available`, `accepted`, `guessed`, `results`, or `expired`). This field
  controls the available chat action and is derived from the authenticated
  viewer's challenge view and guess state.

## Endpoint overview

### Authentication

| Method | Path                           | Auth   | Description                           | Status codes       |
| ------ | ------------------------------ | ------ | ------------------------------------- | ------------------ |
| POST   | `/api/v1/auth/signup`          | No     | Create account                        | 200, 400, 409      |
| POST   | `/api/v1/auth/login`           | No     | Log in                                | 200, 401           |
| POST   | `/api/v1/auth/refresh`         | Cookie | Rotate refresh session                | 200, 401           |
| POST   | `/api/v1/auth/logout`          | No     | Revoke session; `?all=1` revokes all  | 204                |
| POST   | `/api/v1/auth/verify/request`  | Bearer | Send verification email               | 202                |
| POST   | `/api/v1/auth/verify`          | No     | Verify email with `{token}`           | 200, 400           |
| POST   | `/api/v1/auth/password/forgot` | No     | Send reset link `{email}`             | 202                |
| POST   | `/api/v1/auth/password/reset`  | No     | Reset password `{token, password}`    | 200, 400           |
| POST   | `/api/v1/auth/password/change` | Bearer | Change password; revokes all sessions | 204, 400, 401      |
| PATCH  | `/api/v1/auth/profile`         | Bearer | Update username, email, or avatar     | 200, 400, 401, 409 |
| DELETE | `/api/v1/auth/account`         | Bearer | Delete account `{password}`           | 204, 401           |

### Groups

| Method | Path                                              | Auth   | Description                 |
| ------ | ------------------------------------------------- | ------ | --------------------------- |
| GET    | `/api/v1/user/groups`                             | Bearer | List user's groups          |
| POST   | `/api/v1/group/create`                            | Bearer | Create group `{name}`       |
| POST   | `/api/v1/group/join`                              | Bearer | Join group `{code}`         |
| GET    | `/api/v1/group/details?id=`                       | Bearer | Group details (member only) |
| GET    | `/api/v1/group/members?id=`                       | Bearer | List members (member only)  |
| GET    | `/api/v1/group/leaderboard?group_id=`             | Bearer | Leaderboard (member only)   |
| GET    | `/api/v1/group/messages?group_id=&cursor=&limit=` | Bearer | Paginated messages          |

### Challenges

| Method | Path                                          | Auth   | Description                                       |
| ------ | --------------------------------------------- | ------ | ------------------------------------------------- |
| POST   | `/api/v1/photo/upload`                        | Bearer | Upload photo `multipart(photo,group_id,lat,long)` |
| POST   | `/api/v1/challenges/{photoID}/accept`         | Bearer | Accept challenge, start view window               |
| GET    | `/api/v1/challenges/{photoID}/media`          | Bearer | Stream media (during view window)                 |
| GET    | `/api/v1/challenges/{photoID}/media?result=1` | Bearer | Stream media (results visible)                    |
| POST   | `/api/v1/challenges/{photoID}/guess`          | Bearer | Submit guess `{lat, long}`                        |
| GET    | `/api/v1/challenges/{photoID}/results`        | Bearer | Get results                                       |

### WebSocket

| Method | Path                           | Auth   | Description               |
| ------ | ------------------------------ | ------ | ------------------------- |
| POST   | `/api/v1/ws/ticket?group_id=`  | Bearer | Create one-time WS ticket |
| GET    | `/api/v1/ws?group_id=&ticket=` | Ticket | Upgrade to WebSocket chat |

### Health

| Method | Path            | Description                                             |
| ------ | --------------- | ------------------------------------------------------- |
| GET    | `/health/live`  | Liveness (always 200)                                   |
| GET    | `/health/ready` | Readiness (200 if DB + storage OK, 503 otherwise)       |
| GET    | `/metrics`      | Prometheus metrics (Bearer auth required in production) |

## Metrics

Prometheus metrics are available at `/metrics`. The endpoint is public in
`development` and `test` environments. In `production` (and any environment
other than `development` or `test`) the endpoint requires
`Authorization: Bearer <METRICS_TOKEN>`. The presented token is compared to the
configured value in constant time.

A rejected request returns `401` with these response headers:

- `WWW-Authenticate: Bearer` — advertises the required scheme.
- `Cache-Control: no-store` — prevents intermediaries from caching the protected
  response.

The response body is the standard error shape:
`{"error":{"code":"unauthorized","message":"Metrics authentication required"}}`.

Available metrics:

- `geoguessme_http_requests_total` — total HTTP requests
- `geoguessme_http_errors_total` — HTTP 5xx responses
- `geoguessme_storage_cleanup_backlog` — pending object-deletion jobs

## API error codes

| Code                    | Meaning                            |
| ----------------------- | ---------------------------------- |
| `invalid_username`      | Username validation failed         |
| `invalid_email`         | Email validation failed            |
| `invalid_password`      | Password validation failed         |
| `username_taken`        | Username already in use            |
| `email_taken`           | Email already in use               |
| `authentication_failed` | Bad credentials                    |
| `unauthorized`          | Missing or invalid auth            |
| `forbidden`             | Not a member of the required group |
| `not_found`             | Resource not found                 |
| `group_not_found`       | Group not found                    |
| `already_member`        | Already a member                   |
| `invalid_group_name`    | Group name validation failed       |
| `invalid_group_code`    | Group code validation failed       |
| `invalid_upload`        | Upload too large or malformed      |
| `invalid_image`         | Image type or size invalid         |
| `invalid_coordinates`   | Invalid lat/long                   |
| `invalid_request`       | Request body malformed             |
| `challenge_expired`     | Challenge is past its TTL          |
| `viewing_window_open`   | Must wait for view window to end   |
| `media_expired`         | Viewing window has expired         |
| `media_removed`         | Original image no longer available |
| `results_not_available` | Results not yet visible            |
| `origin_not_allowed`    | WebSocket origin rejected          |
| `rate_limited`          | Rate limit exceeded                |
| `internal_error`        | Unexpected server error            |
| `storage_unavailable`   | Media storage unavailable          |
| `storage_error`         | Backend storage error              |
