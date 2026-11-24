# Server Administration Tools

RAMServer provides a small HTTP administration API and the `ramadmin` command-line client.
Administrators can inspect sessions and connections, read recent logs, close abusive resources,
and view server metrics.

The administration API is intentionally separate from the game protocol. Players and
administrators do not share commands or authentication.

## Deployment overview

The repository supports two server modes:

| Mode | Player transport | Admin transport | State |
|---|---|---|---|
| Cloud Run | HTTPS and WebSocket on the port supplied by Cloud Run, normally `8080` | The same HTTPS service at `/v1/*` | In memory; deploy with one instance |
| Docker or VM | TCP on container port `8080` | HTTP on loopback port `8081` | In memory |

Cloud Run is the production-hosting configuration. It is also the primary setup described below.
Port `8081` is used only by the optional Docker/VM listener and was selected because it is adjacent
to, but distinct from, the player port.

The HTTP API is retained because it is a good control-plane format for a stateless command client:
it has clear operations, status codes, and easy TLS protection. It does not need to become part of
the game protocol or use the player's WebSocket framing.

## Security model

- Cloud Run exposes the service over HTTPS.
- A request must include a personal bearer token in `Authorization: Bearer <token>`.
- The server stores only SHA-256 hashes of administrator tokens.
- The token file is shared across staging and production, so an administrator has the same access
  on both services after that administrator is enrolled once.
- Token changes are centralized. There is no need to create a separate administrator for each
  Cloud Run service.
- Failed authentication is throttled and audited.
- Close reasons and player-controlled text are cleaned before they are stored or printed.
- Production and staging remain separate service URLs and separate in-memory state.

Do not commit a token or token hash file. A leaked personal token should be revoked and rotated.

## Cloud Run setup

### 1. Create the shared administrator secret

Each administrator first creates a personal token and shares only the generated hash line:

```bash
cd server
go run ./cmd/ramadmin token alice
```

The command creates a private local token and prints an `alice <64-character-hash>` line.
Keep the token private. Send the hash line to the person who maintains the shared secret.

Create the Secret Manager secret once:

```bash
gcloud secrets create ramserver-admin-tokens \
  --replication-policy=automatic \
  --project=PROJECT_ID
```

Store the administrator hash lines outside the repository:

```text
alice 0123456789abcdef...64_hex_characters
bob   fedcba9876543210...64_hex_characters
```

Save that file locally as `/secure/path/admin-tokens`, then add it as the first secret version:

```bash
gcloud secrets versions add ramserver-admin-tokens \
  --data-file=/secure/path/admin-tokens \
  --project=PROJECT_ID
```

The file format is one `<name> <sha256-hash>` per line. Comments beginning with `#` are allowed.
The server fails closed if the file is missing or malformed.

### 2. Grant the Cloud Run runtime access

Find the runtime service account used by the service:

```bash
gcloud run services describe SERVICE_NAME \
  --region=REGION \
  --project=PROJECT_ID \
  --format='value(spec.template.spec.serviceAccountName)'
```

Grant Secret Manager access:

```bash
gcloud secrets add-iam-policy-binding ramserver-admin-tokens \
  --project=PROJECT_ID \
  --member=serviceAccount:RUNTIME_SERVICE_ACCOUNT \
  --role=roles/secretmanager.secretAccessor
```

The deployment template in `cloudbuild.yaml` mounts the secret at
`/run/secrets/ramserver-admin-tokens` and sets `RAMSERVER_ADMIN_TOKENS_FILE` to that path.

### 3. Deploy

Deploy staging and production with the same secret name and the Cloud Build substitution:

```bash
gcloud builds submit \
  --project=PROJECT_ID \
  --substitutions=_SERVICE_NAME=staging-service,_DEPLOY_REGION=REGION
```

For production, use the production service name and the same `_ADMIN_TOKENS_SECRET` value.
The default in `cloudbuild.yaml` is `ramserver-admin-tokens`.

Cloud Run is configured with one minimum instance and one maximum instance. This keeps the
in-memory session and connection state consistent for close-session and close-connection operations.
Removing this limit would require a shared session registry or instance-aware routing.

## Add or remove an administrator

To add one person:

1. Ask the person to run `ramadmin token <lowercase-name>` on their own machine.
2. Receive the printed `<name> <hash>` line through a secure process.
3. Add the line to the maintained administrator file.
4. Create a new secret version:

   ```bash
   gcloud secrets versions add ramserver-admin-tokens \
     --data-file=/secure/path/admin-tokens \
     --project=PROJECT_ID
   ```

5. Redeploy staging and production if a service's Cloud Run container was already started. The
   mounted secret is read during startup.

The same secret is used by both services, so no per-service administrator registration is needed.
A name is unique, lowercase, and limited to letters, digits, `.`, `_`, or `-`.

To remove someone, remove their line and publish a new secret version. Existing personal tokens stop
working once a container reloads or restarts. Also remove any Google IAM access they may have
separately been granted.

## Run `ramadmin`

Build or run the client from `server/`:

```bash
go run ./cmd/ramadmin -url https://STAGING_URL whoami
go run ./cmd/ramadmin -url https://PRODUCTION_URL sessions
```

Set the URL and token once:

```bash
export RAMSERVER_ADMIN_URL=https://PRODUCTION_URL
export RAMSERVER_ADMIN_TOKEN=ramadm_your_private_token
```

Windows PowerShell:

```powershell
$env:RAMSERVER_ADMIN_URL="https://PRODUCTION_URL"
$env:RAMSERVER_ADMIN_TOKEN="ramadm_your_private_token"
```

The token can also be read from the default private file:

- Linux/macOS: `~/.config/ramserver/admin-token`
- Windows: `%AppData%\ramserver\admin-token`

Use `-token-file` to select a different file.

## Commands

| Command | Result |
|---|---|
| `ramadmin whoami` | Confirms the token and prints the administrator name |
| `ramadmin sessions` | Lists sessions, games, statuses, players, matches, age, and idle time |
| `ramadmin session <id>` | Displays one session and each member's action counts |
| `ramadmin close-session <id> [reason]` | Ends a session and notifies its members |
| `ramadmin connections` | Lists connections, player names, sessions, and message counts |
| `ramadmin close-conn <id>` | Closes a connection and frees its player seat |
| `ramadmin logs -n 200 -grep TEXT` | Lists recent logs, optionally filtered |
| `ramadmin metrics` | Lists counters and current in-memory levels |

Global flags precede commands:

```text
-url URL
-token-file FILE
-json
```

With `-json`, `ramadmin` prints the server response unchanged.

## Docker or VM setup

Docker Compose keeps the game and admin listeners separate:

- Game: host and container port `8080`
- Admin: loopback-only host port `8081` to container port `8081`

Start the VM deployment:

```bash
docker compose -f docker-compose.yml -f docker-compose.admin.yml up -d --build
```

The VM administrator should open an SSH/IAP tunnel:

```bash
gcloud compute ssh VM_NAME \
  --zone=ZONE \
  --project=PROJECT_ID \
  --tunnel-through-iap \
  -- -N -L 8081:127.0.0.1:8081
```

Then use `ramadmin` locally. The default VM URL is `http://127.0.0.1:8081`.

Create the VM token file outside the repository, normally `/etc/ramserver/admin-tokens`.
It must contain at least one valid `<name> <hash>` line and be readable by the container user.
The server watches the file and applies additions or removals without restarting.

| Variable | Purpose |
|---|---|
| `RAMSERVER_ADMIN_ADDR` | VM listener address, such as `127.0.0.1:8081` |
| `RAMSERVER_ADMIN_TOKENS_FILE` | Shared token-hash file path |
| `RAMSERVER_ADMIN_ALLOW_NONLOOPBACK` | Set to `1` only for container networking such as `:8081` |
| `RAMSERVER_ADMIN_URL` | `ramadmin` server URL |
| `RAMSERVER_ADMIN_TOKEN` | `ramadmin` personal bearer token |

Do not map port `8081` to a public host address. Keep it bound to `127.0.0.1`.

## API reference

Every route requires `Authorization: Bearer <token>`. JSON responses and status codes are:

| Method and route | Result |
|---|---|
| `GET /v1/whoami` | `{"admin":"<name>"}` |
| `GET /v1/sessions` | Session list |
| `GET /v1/sessions/{id}` | One session, or `404` |
| `POST /v1/sessions/{id}/close` | End a session; optional `{"reason":"..."}` |
| `GET /v1/connections` | Connection list |
| `POST /v1/connections/{id}/close` | Close a connection |
| `GET /v1/logs?n=100&contains=text` | `{"lines":[...]}` |
| `GET /v1/metrics` | Uptime, counters, and gauges |

Important status codes:

- `401`: missing or invalid token
- `429`: repeated authentication failures
- `404`: unknown session or connection
- `405`: wrong HTTP method
- `504`: an in-memory session did not answer

## Operational notes

- **Stuck session:** use `sessions` to identify `unresponsive`, then close it if necessary.
- **Flooding connection:** use `connections`, identify the large message count or unauthenticated
  connection, then close it.
- **Rejected move:** use `session <id>` to find the member's counts and `logs -grep <session id>` to
  find the reason.
- **Incident history:** recent logs are kept in memory. Cloud Logging or Docker logs remain the
  durable audit source.
- **Scaling:** keep Cloud Run at one instance until state is moved to a shared registry.
- **Multi-region or replica deployments:** the shared token secret works, but in-memory sessions
  and connections remain local to each instance.

## Tests

From `server/`:

```bash
go test -race ./internal/admin/ ./cmd/ramadmin/ ./internal/session/ ./internal/server/
```

Tests cover token and secret-file handling, bearer authentication, throttling, Cloud Run route
mounting, terminal cleaning, session and connection operations, and VM loopback binding.
