package http

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"score-table/internal/db"
	"score-table/internal/match"
	"score-table/internal/views"
)

func TestTeamsOverviewSavedMatchRecords(t *testing.T) {
	ctx := context.Background()
	conn, err := db.OpenAndInit(filepath.Join(t.TempDir(), "teams.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	renderer, err := views.NewRenderer(os.DirFS("../../web/templates"))
	if err != nil {
		t.Fatal(err)
	}
	active := match.NewActiveMatchStore()
	h := NewHandlers(HandlersDeps{DB: conn, Renderer: renderer, Active: active})
	render := func(t *testing.T) string {
		t.Helper()
		response := httptest.NewRecorder()
		h.TeamsOverview(response, httptest.NewRequest(http.MethodGet, "/control_panel/teams", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("Teams overview returned %d: %s", response.Code, response.Body.String())
		}
		return response.Body.String()
	}
	if body := render(t); !strings.Contains(body, "No teams yet.") || !strings.Contains(body, `id="addTeamBtn"`) {
		t.Fatal("empty teams overview lost its message or Add team control")
	}
	teams := []struct {
		id           int64
		name, p1, p2 string
	}{
		{name: "Falcons", p1: "Ava", p2: "Ben"},
		{name: "Falcons", p1: "Cleo", p2: "Dan"},
		{name: "Foxes", p1: "Eli", p2: "Fay"},
		{name: "Newcomers", p1: "Gia", p2: "Hal"},
	}
	for i := range teams {
		team := &teams[i]
		team.id, err = db.CreateTeamWithTwoPlayers(ctx, conn, team.name, team.p1, team.p2)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert := func(first, second, score1, score2 int) int64 {
		t.Helper()
		id, err := db.InsertMatch(ctx, conn, db.InsertMatchParams{
			Team1ID: strconv.FormatInt(teams[first].id, 10), Team1Name: teams[first].name,
			Team2ID: strconv.FormatInt(teams[second].id, 10), Team2Name: teams[second].name,
			Team1Score: score1, Team2Score: score2,
			StartedAt: time.Unix(100, 0), EndedAt: time.Unix(200, 0),
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	firstMatch := insert(0, 1, 5, 1)
	insert(2, 0, 2, 3)
	insert(1, 2, 4, 1)
	drawnMatch := insert(1, 0, 0, 0)
	active.Start(&match.ActiveMatch{
		Team1:           match.TeamSide{TeamID: fmt.Sprint(teams[0].id), Players: []match.PlayerRef{{PlayerID: "live-a"}}},
		Team2:           match.TeamSide{TeamID: fmt.Sprint(teams[3].id), Players: []match.PlayerRef{{PlayerID: "live-b"}}},
		GoalsByPlayerID: map[string]int{"live-a": 99, "live-b": 1},
	})
	idPattern := regexp.MustCompile(`data-team-id="([0-9]+)"`)
	resultPattern := regexp.MustCompile(`<dt>\s*(Wins|Losses|Draws)\s*</dt>\s*<dd>\s*([0-9]+)\s*</dd>`)
	check := func(t *testing.T, want [4][3]int) {
		t.Helper()
		rows := strings.Split(render(t), `<div class="historyItem">`)[1:]
		if len(rows) != len(teams) {
			t.Fatalf("got %d team rows; want %d", len(rows), len(teams))
		}
		seen := map[int64]bool{}
		for _, row := range rows {
			idMatch := idPattern.FindStringSubmatch(row)
			if len(idMatch) != 2 {
				t.Fatal("team row lost its edit button ID")
			}
			id, _ := strconv.ParseInt(idMatch[1], 10, 64)
			if seen[id] {
				t.Fatalf("duplicate team row %d", id)
			}
			seen[id] = true
			index := -1
			for i, team := range teams {
				if team.id == id {
					index = i
				}
			}
			if index < 0 {
				t.Fatalf("unexpected team ID %d", id)
			}
			team := teams[index]
			for _, text := range []string{
				`data-edit-team`, `data-team-name="` + team.name + `"`,
				`<div class="playerChip">` + team.p1 + `</div>`,
				`<div class="playerChip">` + team.p2 + `</div>`,
				fmt.Sprintf(`action="/control_panel/teams/%d/delete"`, id),
			} {
				if !strings.Contains(row, text) {
					t.Fatalf("team %d lost %q", id, text)
				}
			}
			results := map[string]string{}
			for _, result := range resultPattern.FindAllStringSubmatch(row, -1) {
				results[result[1]] = result[2]
			}
			if len(results) != 3 {
				t.Fatalf("team %d needs Wins, Losses and Draws; got %v", id, results)
			}
			for i, label := range []string{"Wins", "Losses", "Draws"} {
				if results[label] != strconv.Itoa(want[index][i]) {
					t.Errorf("team %d %s = %s; want %d", id, label, results[label], want[index][i])
				}
			}
		}
	}
	t.Run("saved results by ID exclude the active match", func(t *testing.T) {
		check(t, [4][3]int{{2, 0, 1}, {1, 1, 1}, {0, 2, 0}, {0, 0, 0}})
	})
	if err := db.UpdateMatchScores(ctx, conn, firstMatch, 0, 6); err != nil {
		t.Fatal(err)
	}
	t.Run("edited scores update both teams", func(t *testing.T) {
		check(t, [4][3]int{{1, 1, 1}, {2, 0, 1}, {0, 2, 0}, {0, 0, 0}})
	})
	if err := db.DeleteMatch(ctx, conn, drawnMatch); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateTeamAndTwoPlayers(ctx, conn, teams[0].id, "Renamed Falcons", "Ava", "Ben"); err != nil {
		t.Fatal(err)
	}
	teams[0].name = "Renamed Falcons"
	t.Run("deleted matches disappear and renamed teams retain results", func(t *testing.T) {
		check(t, [4][3]int{{1, 1, 0}, {2, 0, 0}, {0, 2, 0}, {0, 0, 0}})
	})
}
