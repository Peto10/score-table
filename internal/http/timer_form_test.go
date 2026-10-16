package http

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"score-table/internal/match"
	"score-table/internal/views"
)

func TestTimerFormRetainsSetDuration(t *testing.T) {
	renderer, err := views.NewRenderer(os.DirFS("../../web/templates"))
	if err != nil {
		t.Fatal(err)
	}
	active := match.NewActiveMatchStore()
	active.Start(&match.ActiveMatch{
		Team1:           match.TeamSide{Players: []match.PlayerRef{{PlayerID: "alice"}}},
		GoalsByPlayerID: map[string]int{},
		Timer:           match.Timer{Show: true, DefaultMs: 60000, RemainingMs: 60000},
	})
	h := NewHandlers(HandlersDeps{Active: active, Renderer: renderer, Hub: NewScoreHub()})
	router := chi.NewRouter()
	router.Get("/control_panel/active_match", h.ActiveMatch)
	router.Post("/control_panel/active_match/player/{playerID}/inc", h.PlayerInc)
	router.Post("/control_panel/active_match/timer/reset", h.TimerReset)
	router.Post("/control_panel/active_match/timer/set", h.TimerSet)
	checkFields := func(minutes, seconds int) {
		t.Helper()
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/control_panel/active_match", nil))
		for name, want := range map[string]int{"minutes": minutes, "seconds": seconds} {
			pattern := fmt.Sprintf(`<input\b[^>]*name="%s"[^>]*value="%d"`, name, want)
			if response.Code != http.StatusOK || !regexp.MustCompile(pattern).MatchString(response.Body.String()) {
				t.Fatalf("timer form %s must remain %d (HTTP %d)", name, want, response.Code)
			}
		}
	}
	post := func(action, form string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/control_panel/active_match/"+action, strings.NewReader(form))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusSeeOther {
			t.Fatalf("%s returned HTTP %d", action, response.Code)
		}
	}
	started := time.Now().Add(-30 * time.Second)
	active.TimerSet(started, 195000)
	active.TimerToggleRun(started)
	checkFields(3, 15) // The running countdown is already below the set duration.
	active.TimerToggleRun(started.Add(30 * time.Second))
	checkFields(3, 15) // Paused at 2:45.
	post("player/alice/inc", "")
	checkFields(3, 15)
	post("timer/reset", "")
	checkFields(3, 15)
	post("timer/set", "minutes=4&seconds=20")
	checkFields(4, 20)
}

func TestTimerPanelCanRecoverFromZero(t *testing.T) {
	renderer, err := views.NewRenderer(os.DirFS("../../web/templates"))
	if err != nil {
		t.Fatal(err)
	}
	for _, initial := range []int64{0, 60000} {
		t.Run(fmt.Sprintf("initial_%d", initial), func(t *testing.T) {
			active := match.NewActiveMatchStore()
			active.Start(&match.ActiveMatch{Timer: match.Timer{Show: true, DefaultMs: initial, RemainingMs: initial}})
			h := NewHandlers(HandlersDeps{Active: active, Renderer: renderer, Hub: NewScoreHub()})
			display := httptest.NewRecorder()
			h.DisplayScore(display, httptest.NewRequest(http.MethodGet, "/display_score", nil))
			if display.Code != http.StatusOK || !regexp.MustCompile(`>\s*Paused\s*</span>`).MatchString(display.Body.String()) || strings.Contains(display.Body.String(), "Match paused") {
				t.Fatal("public display must render the Paused caption")
			}
			router := chi.NewRouter()
			router.Get("/control_panel/active_match", h.ActiveMatch)
			router.Post("/control_panel/active_match/timer/set", h.TimerSet)
			router.Post("/control_panel/active_match/timer/toggle", h.TimerToggle)
			router.Post("/control_panel/active_match/timer/reset", h.TimerReset)
			router.Post("/control_panel/active_match/timer/visibility", h.TimerVisibility)
			post := func(action, form string) {
				t.Helper()
				request := httptest.NewRequest(http.MethodPost, "/control_panel/active_match/timer/"+action, strings.NewReader(form))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusSeeOther {
					t.Fatalf("%s returned HTTP %d", action, response.Code)
				}
			}
			checkPanel := func(show bool, minutes, seconds int) {
				t.Helper()
				response := httptest.NewRecorder()
				router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/control_panel/active_match", nil))
				body := response.Body.String()
				if response.Code != http.StatusOK || !strings.Contains(body, `class="timerCard"`) {
					t.Fatal("timer card disappeared")
				}
				for name, want := range map[string]int{"minutes": minutes, "seconds": seconds} {
					field := fmt.Sprintf(`<input\b[^>]*name="%s"[^>]*value="%d"`, name, want)
					if strings.Contains(body, `name="`+name+`"`) != show || (show && !regexp.MustCompile(field).MatchString(body)) {
						t.Fatalf("%s field visibility/value incorrect (show=%v, want=%d)", name, show, want)
					}
				}
				if show {
					for _, action := range []string{"toggle", "reset", "set"} {
						if !strings.Contains(body, `action="/control_panel/active_match/timer/`+action+`"`) {
							t.Fatalf("timer %s control disappeared", action)
						}
					}
				} else if !strings.Contains(body, ">Show timer</button>") {
					t.Fatal("hidden timer cannot be shown again")
				}
				if snap := buildSnapshot(active.Get()); snap.ShowTimer != show || snap.TimerRunning || snap.TimerPaused != show || snap.TimerExpired {
					t.Fatalf("stopped public timer must be paused when visible: %+v", snap)
				}
			}
			checkPanel(true, int(initial/60000), 0)
			post("set", "minutes=0&seconds=0")
			checkPanel(true, 0, 0)
			post("toggle", "")
			checkPanel(true, 0, 0)
			post("reset", "")
			checkPanel(true, 0, 0)
			post("visibility", "show=0")
			checkPanel(false, 0, 0)
			post("visibility", "show=1")
			checkPanel(true, 0, 0)
			post("set", "minutes=1&seconds=30")
			checkPanel(true, 1, 30)
			post("toggle", "")
			if snap := buildSnapshot(active.Get()); !snap.TimerRunning || snap.TimerPaused || snap.TimerExpired {
				t.Fatalf("started public timer must only be running: %+v", snap)
			}
			post("toggle", "")
			if timer := active.Get().Timer; timer.Running || !timer.Paused {
				t.Fatalf("recovered timer could not pause: %+v", timer)
			}
			checkPanel(true, 1, 30)
			post("reset", "")
			if timer := active.Get().Timer; timer.Running || timer.Paused || timer.RemainingMs != 90000 {
				t.Fatalf("recovered timer could not reset: %+v", timer)
			}
			checkPanel(true, 1, 30)
		})
	}
}
