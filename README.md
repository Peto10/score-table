# Floorball score table

Two-window localhost app:
- **Display window** (for audience): `GET /display_score` (updates live via SSE, no refresh)
- **Control panel** (admin): `GET /control_panel`

Backend is **Go**, storage is **SQLite**, teams/players come from a **YAML** config.

## Run (Docker)

```bash
docker compose up --build
```

If port 8080 is already in use, pick a different host port:

```bash
HOST_PORT=8090 docker compose up --build
```

Open:
- `http://localhost:8080/display_score` (or `http://localhost:8090/display_score` if you changed `HOST_PORT`)
- `http://localhost:8080/control_panel` (or `http://localhost:8090/control_panel` if you changed `HOST_PORT`)

Data is stored in `./data/app.db` (mounted into the container at `/data/app.db`).

## Run (local Go)

```bash
mkdir -p data
go run ./cmd/server -addr :8080 -config ./config/teams.yaml -db ./data/app.db
```

## Teams/players config

Edit `[config/teams.yaml](config/teams.yaml)`.

Format:

```yaml
teams:
  - id: team_a
    name: "TEAM A"
    players:
      - id: alice
        name: "Alice"
      - id: bob
        name: "Bob"
  - id: team_b
    name: "TEAM B"
    players:
      - id: carl
        name: "Carl"
      - id: dana
        name: "Dana"
```

Notes:
- Team and player **IDs must be unique**.
- Team names can be long; the UI clamps them to **2 lines**.

