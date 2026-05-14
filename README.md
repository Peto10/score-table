# score table

Two-window app to run on localhost:
- **Display (public scoreboard)**: `/display_score`
- **Control panel (admin)**: `/control_panel`

## Quick start (Docker)

```bash
git clone git@github.com:Peto10/Score-Table.git
cd Score-Table
```

```bash
docker compose up --build
```

If `8080` is busy, try different port (eg. `HOST_PORT=8090 docker compose up --build`)

Open:
- `http://localhost:8080/display_score` (or `http://localhost:8090/display_score`)
- `http://localhost:8080/control_panel` (or `http://localhost:8090/control_panel`)

Data about teams, score, players, timer defaults etc. persists in `./data/app.db` (SQLite).

## Quick test data add

Warning: this will erase your in-app data (teams, players, scores, timer defaults...).
```bash
# BE CAREFUL WITH rm -rf
# Your working directory must be the root directory of this project
rm -rf data && cp -r template_data data
```


## Functionality

- **Teams**: add/edit/delete in `http://localhost:8080/control_panel/teams`
- **Timer defaults**: set in `http://localhost:8080/control_panel/settings`
- **Live scoreboard**: `/display_score` updates automatically (no refresh).
- **Start match**: pick two teams and start from `/control_panel`.
- **Add/remove goals**: `+` / `-` per player in active match.
- **Store matches**: writes match + goal rows into SQLite.
- **Timer**: start/pause, reset, set minutes/seconds, show/hide.
- **Swap display sides**: flip left/right teams on the public display.
- **Match history**: list saved matches and delete them (with confirmation).
- **Edit match**: open a saved match and adjust goals.
- **Player stats matrix**: player goals per opponent team + TOTAL.

