# Admin Web

`admin-web/` is the project-level management console. It is served by
`backend` at `/dashboard/` and authenticates via session cookies created
by `POST /api/auth/login`.

Runtime lookup order in `backend/apis/router.go` (`resolveAdminWebDir`):

1. `./admin-web`
2. `../admin-web`
3. `../../admin-web`
4. `./web`
5. `../web`
6. `/opt/admin-web` (Docker default)

The last two `./web` / `../web` entries are legacy fallbacks only.

The Stats view exposes DB-backed settings for the WebRTC play domain and
the per-environment public base URLs (`local`, `dev`, `test`, `stag`,
`prod`). All values persist in the MySQL `info_system_settings` table.
Legacy `uat` env input is accepted and normalized to `dev` server-side.

Build the backend Docker image from the repository root so the build
context contains both `backend/` and `admin-web/`:

```bash
docker build -t abrplayer-backend:v1.0.0 -f Dockerfile .
```
