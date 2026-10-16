package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"score-table/internal/match"
	"score-table/internal/views"
)

func TestActiveMatchMirrorsPublicDisplayThroughSideSwaps(t *testing.T) {
	renderer, err := views.NewRenderer(os.DirFS("../../web/templates"))
	if err != nil {
		t.Fatal(err)
	}
	teams := []match.TeamSide{
		{TeamID: "maple", TeamName: "Maple", Players: []match.PlayerRef{
			{PlayerID: "maple-a", PlayerName: "Alice Maple"},
			{PlayerID: "maple-b", PlayerName: "Bea Maple"},
		}},
		{TeamID: "oak", TeamName: "Oak", Players: []match.PlayerRef{
			{PlayerID: "oak-a", PlayerName: "Clara Oak"},
			{PlayerID: "oak-b", PlayerName: "Dora Oak"},
		}},
	}
	active := match.NewActiveMatchStore()
	active.Start(&match.ActiveMatch{
		Team1: teams[0], Team2: teams[1], StartedAt: time.Unix(100, 0),
		GoalsByPlayerID: map[string]int{"maple-a": 3, "maple-b": 1, "oak-a": 2, "oak-b": 5},
	})
	hub := NewScoreHub()
	events := hub.Subscribe()
	defer hub.Unsubscribe(events)
	h := NewHandlers(HandlersDeps{Active: active, Renderer: renderer, Hub: hub})
	router := chi.NewRouter()
	router.Get("/control_panel/active_match", h.ActiveMatch)
	router.Post("/control_panel/active_match/display/swap_sides", h.SwapDisplaySides)
	router.Post("/control_panel/active_match/player/{playerID}/inc", h.PlayerInc)
	router.Post("/control_panel/active_match/player/{playerID}/dec", h.PlayerDec)

	for _, step := range []struct {
		name    string
		post    string
		swapped bool
		goals   [4]int
	}{
		{"initial", "", false, [4]int{3, 1, 2, 5}},
		{"first swap", "display/swap_sides", true, [4]int{3, 1, 2, 5}},
		{"increment after swap", "player/maple-a/inc", true, [4]int{4, 1, 2, 5}},
		{"decrement after swap", "player/oak-b/dec", true, [4]int{4, 1, 2, 4}},
		{"second swap", "display/swap_sides", false, [4]int{4, 1, 2, 4}},
	} {
		t.Run(step.name, func(t *testing.T) {
			publicLeft, publicRight := 0, 1
			if step.swapped {
				publicLeft, publicRight = 1, 0
			}
			scores := [2]int{step.goals[0] + step.goals[1], step.goals[2] + step.goals[3]}
			checkPublicEvent := func(event []byte) {
				t.Helper()
				data := strings.TrimSuffix(strings.TrimPrefix(string(event), "event: snapshot\ndata: "), "\n\n")
				var snap snapshot
				if err := json.Unmarshal([]byte(data), &snap); err != nil {
					t.Fatalf("invalid public snapshot: %v", err)
				}
				if !snap.Active || snap.Team1Name != teams[publicLeft].TeamName || snap.Team2Name != teams[publicRight].TeamName || snap.Team1Score != scores[publicLeft] || snap.Team2Score != scores[publicRight] {
					t.Fatalf("public sides or scores incorrect: %+v", snap)
				}
			}
			if step.post != "" {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/control_panel/active_match/"+step.post, nil))
				if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/control_panel/active_match" {
					t.Fatalf("action response = %d, location %q", response.Code, response.Header().Get("Location"))
				}
				select {
				case event := <-events:
					checkPublicEvent(event)
				default:
					t.Fatal("action did not broadcast the public snapshot")
				}
			}
			// A new audience connection and a subsequent panel GET must agree.
			event, ok := h.snapshotEvent()
			if !ok {
				t.Fatal("active match has no public snapshot")
			}
			checkPublicEvent(event)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/control_panel/active_match", nil))
			if response.Code != http.StatusOK {
				t.Fatalf("panel response = %d", response.Code)
			}
			body := response.Body.String()
			left, right := publicRight, publicLeft
			if !strings.Contains(body, fmt.Sprintf(`<h1 class="panelTitle">%s vs %s</h1>`, teams[left].TeamName, teams[right].TeamName)) {
				t.Fatal("panel title does not mirror the public sides")
			}
			inline := regexp.MustCompile(`<div class="scoreInline(?:Team[^"]*|Num)">([^<]*)</div>`).FindAllStringSubmatch(body, -1)
			var gotInline []string
			for _, field := range inline {
				gotInline = append(gotInline, field[1])
			}
			wantInline := []string{teams[left].TeamName, fmt.Sprint(scores[left]), fmt.Sprint(scores[right]), teams[right].TeamName}
			if !reflect.DeepEqual(gotInline, wantInline) {
				t.Fatalf("panel scoreboard = %v; want %v", gotInline, wantInline)
			}
			playerSection := regexp.MustCompile(`(?s)<section class="twoCol">(.*?)</section>`).FindStringSubmatch(body)
			if len(playerSection) != 2 {
				t.Fatal("panel player cards missing")
			}
			cards := strings.Split(playerSection[1], `<div class="card">`)
			if len(cards) != 3 {
				t.Fatalf("panel has %d player cards; want 2", len(cards)-1)
			}
			for side, teamIndex := range []int{left, right} {
				card := cards[side+1]
				if !strings.Contains(card, fmt.Sprintf(`<h2 class="sectionTitle teamClamp">%s</h2>`, teams[teamIndex].TeamName)) {
					t.Fatalf("player card %d does not match its scoreboard team", side)
				}
				players := regexp.MustCompile(`<div class="playerName">([^<]*)</div>`).FindAllStringSubmatch(card, -1)
				goals := regexp.MustCompile(`<div class="playerGoals">([^<]*)</div>`).FindAllStringSubmatch(card, -1)
				if len(players) != 2 || len(goals) != 2 {
					t.Fatalf("player card %d has incorrect player or goal count", side)
				}
				for playerIndex, player := range teams[teamIndex].Players {
					key := side*2 + playerIndex + 1
					if players[playerIndex][1] != fmt.Sprintf("%s (%d)", player.PlayerName, key) || goals[playerIndex][1] != fmt.Sprint(step.goals[teamIndex*2+playerIndex]) {
						t.Fatalf("player card %d lost player/goal association for %s", side, player.PlayerID)
					}
					shortcut := fmt.Sprintf(`action="/control_panel/active_match/player/%s/inc">\s*<button[^>]*data-shortcut="%d"[^>]*aria-keyshortcuts="%d"[^>]*>\+</button>`, player.PlayerID, key, key)
					if !regexp.MustCompile(shortcut).MatchString(card) {
						t.Fatalf("player %s must have shortcut %d on its plus button", player.PlayerID, key)
					}
					for _, action := range []string{"inc", "dec"} {
						if !strings.Contains(card, fmt.Sprintf(`action="/control_panel/active_match/player/%s/%s"`, player.PlayerID, action)) {
							t.Fatalf("player %s lost %s control", player.PlayerID, action)
						}
					}
				}
			}
			stored := active.Get()
			wantGoals := map[string]int{"maple-a": step.goals[0], "maple-b": step.goals[1], "oak-a": step.goals[2], "oak-b": step.goals[3]}
			if !reflect.DeepEqual(stored.Team1, teams[0]) || !reflect.DeepEqual(stored.Team2, teams[1]) || !reflect.DeepEqual(stored.GoalsByPlayerID, wantGoals) || stored.DisplaySwap != step.swapped {
				t.Fatalf("presentation changed stored teams or player goal association: %+v", stored)
			}
		})
	}
}
