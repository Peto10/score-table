# Score table

Two-window app to run on localhost:
- **Display (public scoreboard)**: `/display_score`
- **Control panel (admin)**: `/control_panel`

## Demo video
https://github.com/user-attachments/assets/74c383bc-db6b-47b5-9587-179eeb57f159

## Quick start (Docker)

```bash
git clone git@github.com:Peto10/score-table.git
cd score-table
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

- **Teams**: add/edit/delete in `http://localhost:8080/control_panel/teams`, with wins, losses, and draws from saved matches.
- **Optional team groups**: create or delete groups in Teams overview and choose a group when adding or editing a team. Grouped teams appear first, followed by teams without a group. Deleting a group keeps its teams. Groups only organize this overview; they do not affect team selection or matches.
- **Timer defaults**: set in `http://localhost:8080/control_panel/settings`
- **Live scoreboard**: `/display_score` updates automatically (no refresh).
- **Start match**: pick two teams and start from `/control_panel`.
- **Add/remove goals**: `+` / `-` per player in active match.
- **Store matches**: writes match + goal rows into SQLite.
- **Timer**: start/pause, reset, set minutes/seconds, show/hide.
- **Keyboard (active match)**: `P` starts/pauses the timer; `1`–`4` add a goal to the players in control panel order. Shortcuts are disabled while editing inputs.
- **Swap sides**: flip teams in both views; the control panel always mirrors the public display.
- **Match history**: list saved matches with highlighted winners or draws, scores centered below each team, and deletion with confirmation.
- **Edit match**: open a saved match and adjust goals.
- **Player stats matrix**: player goals per opponent team + TOTAL.

**Note**: this project is half-vibed, so please don’t judge the code.
