# Server Administration Tools

RAMServer provides an HTTP administration API and the `ramadmin` command-line client. Administrators can inspect sessions and connections, read recent logs, close abusive resources, and view server metrics.

The administration API is separate from the game protocol. Players and administrators use different commands and authentication.

## Deployment overview

| Mode | Player transport | Admin transport | State |
|------|------------------|-----------------|-------|
| Cloud Run | HTTPS and WebSocket on the port supplied by Cloud Run, normally `8080` | The same HTTPS service at `/v1/*` | In memory; use one instance until state is shared |
| Docker or VM | TCP on container port `8080` | HTTP on loopback port `8081` | In memory |

Cloud Run is the primary hosted deployment. Port `8081` is used only by the optional Docker/VM listener.

HTTP is used for the administration API because it provides clear request/response operations, status codes, JSON responses, bearer authentication, TLS support, and straightforward command-line tooling. Administration should not be added to the player protocol or game WebSocket framing.

## Security model

- Cloud Run exposes the service over HTTPS.
- Every admin request must include `Authorization: Bearer <token>`.
- The server stores only SHA-256 hashes of administrator tokens.
- The shared token secret is used by staging and production, so an administrator needs to be enrolled only once.
- Failed authentication is throttled and audited.
- Close reasons and player-controlled text are cleaned before being logged or printed.
- Staging and production have separate URLs and separate in-memory state.

Never commit an administrator token or the maintained hash file. If a token is exposed, revoke it and issue a new token.

## Administrator quick start

Use this section if you are a team member requesting administrator access.

### 1. Create a personal administrator token

From the repository, make sure Go 1.24 or later is installed. Then run:

```bash
cd server
go run ./cmd/ramadmin token YOUR_NAME
```

On Windows:

```powershell
cd server
go run ./cmd/ramadmin token YOUR_NAME
```

Replace `YOUR_NAME` with your administrator name. Names must be unique and use only lowercase letters, digits, `.`, `_`, or `-`. The name can be as short as one character and no longer than 32 characters.

For example:

```bash
go run ./cmd/ramadmin token alice
```

The command creates a private token file at:

- Linux: `~/.config/ramserver/admin-token`
- macOS: `~/.config/ramserver/admin-token`
- Windows: `%AppData%\ramserver\admin-token`

The command also prints one line like:

```text
alice 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

This line contains your administrator name and the hash of your personal token. It does not contain the token itself.

### 2. Send the line to the administrator owner

Send the complete hash line to the person who maintains RAMServer administration. Send it through a trusted channel approved by your team.

```text
alice 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

Do not send the contents of `admin-token`. Do not paste the personal token into chat, an issue, a pull request, or a source file.

You do not need to rerun `ramadmin token` unless the owner tells you to rotate or replace the token.

### 3. Confirm access

After the owner applies your entry and deploys the new revision, set the service URL and personal token.

Linux or macOS:

```bash
export RAMSERVER_ADMIN_URL=https://YOUR_SERVICE_URL
export RAMSERVER_ADMIN_TOKEN=$(cat ~/.config/ramserver/admin-token)
go run ./cmd/ramadmin whoami
```

Windows PowerShell:

```powershell
$env:RAMSERVER_ADMIN_URL="https://YOUR_SERVICE_URL"
$env:RAMSERVER_ADMIN_TOKEN=(Get-Content "$env:APPDATA\ramserver\admin-token")
go run ./cmd/ramadmin whoami
```

Alternatively, pass the URL directly:

```bash
go run ./cmd/ramadmin -url https://YOUR_SERVICE_URL whoami
```

A successful result is:

```text
authenticated as alice
```

If the command returns unauthorized or too many requests, wait for the owner to confirm that the hash line was added and a new service revision was deployed.

## Adding or removing team administrators

Use this section if you maintain the shared administrator list.

### Maintain the administrator file

The administrator file contains only names and token hashes. Keep it outside the repository.

Suggested locations:

- Linux or macOS: `~/.config/ramserver/admin-tokens`
- Windows: `%AppData%\ramserver\admin-tokens`
- VM: `/etc/ramserver/admin-tokens`

The file should contain one entry per line:

```text
alice 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
bob fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210
```

Do not add personal tokens to this file. Do not commit it.

On Windows, create the file in PowerShell:

```powershell
New-Item -ItemType Directory -Force "$env:APPDATA\ramserver"
notepad "$env:APPDATA\ramserver\admin-tokens"
```

On Linux or macOS:

```bash
mkdir -p ~/.config/ramserver
chmod 700 ~/.config/ramserver
nano ~/.config/ramserver/admin-tokens
chmod 600 ~/.config/ramserver/admin-tokens
```

### Add an administrator

1. Ask the team member to create a token by running `go run ./cmd/ramadmin token THEIR_NAME`.
2. Ask them to send the complete `<name> <hash>` line from the command output.
3. Verify that the name is valid and that it is not already present.
4. Append the line to the maintained administrator file.
5. Preserve every existing line. The file is cumulative.
6. Publish the complete file as a new Secret Manager version.
7. Redeploy both staging and production so they load the new version.

Publish the file to the shared Cloud Run secret:

```bash
gcloud secrets versions add ramserver-admin-tokens \
  --data-file=PATH_TO_ADMIN_TOKENS \
  --project=PROJECT_ID
```

Examples:

```bash
gcloud secrets versions add ramserver-admin-tokens \
  --data-file=~/.config/ramserver/admin-tokens \
  --project=ramserver-core
```

```powershell
gcloud secrets versions add ramserver-admin-tokens `
  --data-file="$env:APPDATA\ramserver\admin-tokens" `
  --project=ramserver-core
```

The staged and production services use the same secret, so no separate team registration is needed for either service. Redeploying from the same Cloud Build configuration mounts the secret in both services.

### Remove an administrator

1. Remove only that person's line from the maintained administrator file.
2. Preserve every other line.
3. Publish the updated file as a new Secret Manager version.
4. Redeploy staging and production.
5. Revoke any separate Google IAM access granted to that person.

Removing a Secret Manager entry does not immediately alter an already running container. Redeploy or restart the service before assuming the token has been revoked.

## Initial Cloud Run setup

This is a one-time owner setup for a project that has not yet configured the shared administrator secret.

### 1. Enable Secret Manager

```bash
gcloud services enable secretmanager.googleapis.com --project=PROJECT_ID
```

Then create the secret:

```bash
gcloud secrets create ramserver-admin-tokens \
  --replication-policy=automatic \
  --project=PROJECT_ID
```

Create the maintained administrator file locally, add at least one `<name> <hash>` line, and upload it:

```bash
gcloud secrets versions add ramserver-admin-tokens \
  --data-file=PATH_TO_ADMIN_TOKENS \
  --project=PROJECT_ID
```

### 2. Grant the Cloud Run runtime access

Find the runtime service account:

```bash
gcloud run services describe SERVICE_NAME \
  --region=REGION \
  --project=PROJECT_ID \
  --format='value(spec.template.spec.serviceAccountName)'
```

If the service uses the default Compute Engine service account, it may look like `123456789-compute@developer.gserviceaccount.com`.

Grant only Secret Manager access:

```bash
gcloud secrets add-iam-policy-binding ramserver-admin-tokens \
  --project=PROJECT_ID \
  --member=serviceAccount:RUNTIME_SERVICE_ACCOUNT \
  --role=roles/secretmanager.secretAccessor
```

The Cloud Build deployment mounts the secret at `/run/secrets/ramserver-admin-tokens` and sets `RAMSERVER_ADMIN_TOKENS_FILE` to that path.

### 3. Deploy staging and production

```bash
gcloud builds submit \
  --project=PROJECT_ID \
  --substitutions=_SERVICE_NAME=ramserver-core-staging,_DEPLOY_REGION=REGION
```

Deploy production with the production service name:

```bash
gcloud builds submit \
  --project=PROJECT_ID \
  --substitutions=_SERVICE_NAME=ramserver-core-production,_DEPLOY_REGION=REGION
```

The default `_ADMIN_TOKENS_SECRET` substitution is `ramserver-admin-tokens`. If the secret name changes, set `_ADMIN_TOKENS_SECRET` to the new name in both deployments.

Cloud Run should keep one minimum instance and one maximum instance while sessions and connections are stored in memory. Scaling or replica routing will require a shared session registry.

## Run `ramadmin`

Build or run the client from `server/`:

```bash
go run ./cmd/ramadmin -url https://STAGING_URL whoami
go run ./cmd/ramadmin -url https://PRODUCTION_URL sessions
```

Set the service URL and personal token once:

Linux or macOS:

```bash
export RAMSERVER_ADMIN_URL=https://PRODUCTION_URL
export RAMSERVER_ADMIN_TOKEN=$(cat ~/.config/ramserver/admin-token)
```

Windows PowerShell:

```powershell
$env:RAMSERVER_ADMIN_URL="https://PRODUCTION_URL"
$env:RAMSERVER_ADMIN_TOKEN=(Get-Content "$env:APPDATA\ramserver\admin-token")
```

Use `-token-file` to select a different private token file.

## Commands

| Command                                | Result                                                                |
|----------------------------------------|-----------------------------------------------------------------------|
| `ramadmin whoami`                      | Confirms the token and prints the administrator name                  |
| `ramadmin sessions`                    | Lists sessions, games, statuses, players, matches, age, and idle time |
| `ramadmin session <id>`                | Displays one session and each member's action counts                  |
| `ramadmin close-session <id> [reason]` | Ends a session and notifies its members                               |
| `ramadmin connections`                 | Lists connections, player names, sessions, and message counts         |
| `ramadmin close-conn <id>`             | Closes a connection and frees its player seat                         |
| `ramadmin logs -n 200 -grep TEXT`      | Lists recent logs, optionally filtered                                |
| `ramadmin metrics`                     | Lists counters and current in-memory levels                           |

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

Create the VM token file outside the repository, normally `/etc/ramserver/admin-tokens`. It must contain at least one valid `<name> <hash>` line and be readable by the container user. The server watches the file and applies additions or removals without restarting.

| Variable                            | Purpose                                                  |
|-------------------------------------|----------------------------------------------------------|
| `RAMSERVER_ADMIN_ADDR`              | VM listener address, such as `127.0.0.1:8081`            |
| `RAMSERVER_ADMIN_TOKENS_FILE`       | Shared token-hash file path                              |
| `RAMSERVER_ADMIN_ALLOW_NONLOOPBACK` | Set to `1` only for container networking such as `:8081` |
| `RAMSERVER_ADMIN_URL`               | `ramadmin` server URL                                    |
| `RAMSERVER_ADMIN_TOKEN`             | `ramadmin` personal bearer token                         |

Do not map port `8081` to a public host address. Keep it bound to `127.0.0.1`.

## API reference

Every route requires `Authorization: Bearer <token>`. JSON responses and status codes are:

| Method and route                   | Result                                     |
|------------------------------------|--------------------------------------------|
| `GET /v1/whoami`                   | `{"admin":"<name>"}`                       |
| `GET /v1/sessions`                 | Session list                               |
| `GET /v1/sessions/{id}`            | One session, or `404`                      |
| `POST /v1/sessions/{id}/close`     | End a session; optional `{"reason":"..."}` |
| `GET /v1/connections`              | Connection list                            |
| `POST /v1/connections/{id}/close`  | Close a connection                         |
| `GET /v1/logs?n=100&contains=text` | `{"lines":[...]}`                          |
| `GET /v1/metrics`                  | Uptime, counters, and gauges               |

Important status codes:

- `401`: missing or invalid token
- `429`: repeated authentication failures
- `404`: unknown session or connection
- `405`: wrong HTTP method
- `504`: an in-memory session did not answer

## Operational notes

- **Stuck session:** use `sessions` to identify `unresponsive`, then close it if necessary.
- **Flooding connection:** use `connections`, identify the large message count or unauthenticated connection, then close it.
- **Rejected move:** use `session <id>` to find the member's counts and `logs -grep <session id>` to find the reason.
- **Incident history:** recent logs are kept in memory. Cloud Logging or Docker logs remain the durable audit source.
- **Scaling:** keep Cloud Run at one instance until state is moved to a shared registry.
- **Multi-region or replica deployments:** the shared token secret works, but in-memory sessions and connections remain local to each instance.

## Tests

From `server/`:

```bash
go test -race ./internal/admin/ ./cmd/ramadmin/ ./internal/session/ ./internal/server/
```

Tests cover token and secret-file handling, bearer authentication, throttling, Cloud Run route mounting, terminal cleaning, session and connection operations, and VM loopback binding.
