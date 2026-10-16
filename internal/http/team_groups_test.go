package http

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"score-table/internal/db"
	"score-table/internal/match"
	"score-table/internal/views"
)

type teamGroupsFixture struct {
	t      *testing.T
	conn   *sql.DB
	h      *Handlers
	router *chi.Mux
	active *match.Store
	hub    *ScoreHub
}

func newTeamGroupsFixture(t *testing.T) *teamGroupsFixture {
	t.Helper()
	conn, err := db.OpenAndInit(filepath.Join(t.TempDir(), "groups.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	renderer, err := views.NewRenderer(os.DirFS("../../web/templates"))
	if err != nil {
		t.Fatal(err)
	}
	f := &teamGroupsFixture{t: t, conn: conn, active: match.NewActiveMatchStore(), hub: NewScoreHub(), router: chi.NewRouter()}
	f.h = NewHandlers(HandlersDeps{DB: conn, Renderer: renderer, Active: f.active, Hub: f.hub})
	f.router.Get("/control_panel", f.h.ControlPanel)
	f.router.Get("/control_panel/teams", f.h.TeamsOverview)
	f.router.Get("/control_panel/active_match", f.h.ActiveMatch)
	f.router.Post("/control_panel/start_match", f.h.StartMatch)
	f.router.Post("/control_panel/teams/create", f.h.CreateTeam)
	f.router.Post("/control_panel/teams/{teamID}/update", f.h.UpdateTeam)
	f.router.Post("/control_panel/teams/groups/create", f.h.CreateTeamGroup)
	f.router.Post("/control_panel/teams/groups/{groupID}/delete", f.h.DeleteTeamGroup)
	return f
}

func (f *teamGroupsFixture) get(path string) string {
	f.t.Helper()
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusOK {
		f.t.Fatalf("GET %s returned %d: %s", path, w.Code, w.Body.String())
	}
	return w.Body.String()
}

func (f *teamGroupsFixture) post(path string, form url.Values, wantError bool) {
	f.t.Helper()
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	location, err := url.Parse(w.Header().Get("Location"))
	if err != nil || w.Code != http.StatusSeeOther || location == nil || location.Query().Has("err") != wantError {
		f.t.Fatalf("POST %s returned %d, location %q; want redirect with error=%v", path, w.Code, w.Header().Get("Location"), wantError)
	}
}

func (f *teamGroupsFixture) group(name string) int64 {
	f.t.Helper()
	f.post("/control_panel/teams/groups/create", url.Values{"group_name": {name}}, false)
	groups, err := db.ListTeamGroups(context.Background(), f.conn)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, group := range groups {
		if group.Name == name {
			return group.ID
		}
	}
	f.t.Fatalf("created group %q missing", name)
	return 0
}

func groupTeamForm(name string) url.Values {
	return url.Values{"team_name": {name}, "player1": {name + " one"}, "player2": {name + " two"}}
}

func (f *teamGroupsFixture) team(name string, groupID int64) db.TeamWithPlayers {
	f.t.Helper()
	form := groupTeamForm(name)
	if groupID > 0 {
		form.Set("group_id", strconv.FormatInt(groupID, 10))
	}
	f.post("/control_panel/teams/create", form, false)
	teams, err := db.ListTeamsWithPlayers(context.Background(), f.conn)
	if err != nil {
		f.t.Fatal(err)
	}
	for _, team := range teams {
		if team.Name == name {
			return team
		}
	}
	f.t.Fatalf("created team %q missing", name)
	return db.TeamWithPlayers{}
}

func (f *teamGroupsFixture) update(team db.TeamWithPlayers, groupID string) {
	f.t.Helper()
	form := groupTeamForm(team.Name)
	form.Set("group_id", groupID)
	f.post(fmt.Sprintf("/control_panel/teams/%d/update", team.ID), form, false)
}

func (f *teamGroupsFixture) membership(teamID, wantGroupID int64) {
	f.t.Helper()
	memberships, err := db.ListTeamGroupMemberships(context.Background(), f.conn)
	if err != nil {
		f.t.Fatal(err)
	}
	if memberships[teamID] != wantGroupID {
		f.t.Fatalf("team %d group = %d; want %d", teamID, memberships[teamID], wantGroupID)
	}
}

func TestTeamGroupsHTTPAssignmentAndDeletion(t *testing.T) {
	f := newTeamGroupsFixture(t)
	ctx := context.Background()
	empty := f.get("/control_panel/teams")
	if !strings.Contains(empty, "No teams yet.") || !strings.Contains(empty, `name="group_id"`) {
		t.Fatal("zero groups must preserve team creation with an optional group field")
	}
	a, b, unused := f.group("A"), f.group("B"), f.group("Empty")
	first := f.team("Wolves", a)
	second := f.team("Dragons", b)
	ungrouped := f.team("Foxes", 0)
	f.membership(first.ID, a)
	f.membership(second.ID, b)
	f.membership(ungrouped.ID, 0)

	sections := regexp.MustCompile(`(?s)<section\b[^>]*data-group-id="([^"]*)"[^>]*>(.*?)</section>`).FindAllStringSubmatch(f.get("/control_panel/teams"), -1)
	if len(sections) != 4 || sections[len(sections)-1][1] != "" {
		t.Fatalf("all three groups, including an empty one, must render before ungrouped teams; got %d sections", len(sections))
	}
	wantTeams := map[string]int64{fmt.Sprint(a): first.ID, fmt.Sprint(b): second.ID, fmt.Sprint(unused): 0, "": ungrouped.ID}
	for _, section := range sections {
		teamID, ok := wantTeams[section[1]]
		if !ok {
			t.Fatalf("unexpected group section %q", section[1])
		}
		delete(wantTeams, section[1])
		if teamID == 0 {
			if strings.Contains(section[2], "data-team-id=") || !strings.Contains(section[2], "Empty") {
				t.Fatal("empty group must remain visible without inventing a team")
			}
			continue
		}
		button := regexp.MustCompile(fmt.Sprintf(`(?s)<button\b[^>]*data-team-id="%d"[^>]*>`, teamID)).FindString(section[2])
		if button == "" || !strings.Contains(button, `data-group-id="`+section[1]+`"`) || strings.Count(section[2], "data-team-id=") != 1 {
			t.Fatalf("team %d is absent from its group or its editor lost the current membership", teamID)
		}
	}
	if len(wantTeams) != 0 {
		t.Fatalf("missing group sections: %v", wantTeams)
	}
	groupSelect := regexp.MustCompile(`<select\b[^>]*name="group_id"[^>]*>`).FindString(empty)
	if groupSelect == "" || regexp.MustCompile(`\brequired\b`).MatchString(groupSelect) {
		t.Fatal("team group selection must be optional even with no groups")
	}

	f.update(first, fmt.Sprint(b))
	f.membership(first.ID, b)
	f.update(first, "")
	f.membership(first.ID, 0)
	f.update(first, fmt.Sprint(a))
	f.membership(first.ID, a)

	matchID, err := db.InsertMatch(ctx, f.conn, db.InsertMatchParams{
		Team1ID: fmt.Sprint(first.ID), Team1Name: first.Name, Team2ID: fmt.Sprint(second.ID), Team2Name: second.Name,
		Team1Score: 3, Team2Score: 1, StartedAt: time.Unix(100, 0), EndedAt: time.Unix(200, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.InsertMatchPlayerGoals(ctx, f.conn, matchID, []db.PlayerGoalRow{{
		PlayerID: fmt.Sprint(first.Players[0].ID), PlayerName: first.Players[0].Name,
		ScoringTeamID: fmt.Sprint(first.ID), ScoringTeamName: first.Name,
		OpponentTeamID: fmt.Sprint(second.ID), OpponentTeamName: second.Name, Goals: 3,
	}}); err != nil {
		t.Fatal(err)
	}
	beforeTeams, _ := db.ListTeamsWithPlayers(ctx, f.conn)
	beforeMatches, _ := db.ListMatches(ctx, f.conn)
	beforeGoals, _ := db.ListMatchPlayerGoals(ctx, f.conn, matchID)
	beforeRecords, _ := db.ListTeamRecords(ctx, f.conn)
	f.post(fmt.Sprintf("/control_panel/teams/groups/%d/delete", a), nil, false)
	f.membership(first.ID, 0)
	afterTeams, err := db.ListTeamsWithPlayers(ctx, f.conn)
	if err != nil || !reflect.DeepEqual(beforeTeams, afterTeams) {
		t.Fatalf("deleting an occupied group changed teams or player IDs: %v", err)
	}
	afterMatches, err := db.ListMatches(ctx, f.conn)
	if err != nil || !reflect.DeepEqual(beforeMatches, afterMatches) {
		t.Fatalf("deleting a group changed match history: %v", err)
	}
	afterGoals, err := db.ListMatchPlayerGoals(ctx, f.conn, matchID)
	if err != nil || !reflect.DeepEqual(beforeGoals, afterGoals) {
		t.Fatalf("deleting a group changed saved player goals: %v", err)
	}
	afterRecords, err := db.ListTeamRecords(ctx, f.conn)
	if err != nil || !reflect.DeepEqual(beforeRecords, afterRecords) {
		t.Fatalf("deleting a group changed win/loss/draw records: %v", err)
	}
	// A form opened before deletion can still save the team; the deleted ID becomes no group.
	stale := groupTeamForm("Renamed Wolves")
	stale.Set("group_id", fmt.Sprint(a))
	f.post(fmt.Sprintf("/control_panel/teams/%d/update", first.ID), stale, false)
	f.membership(first.ID, 0)
	updated, err := db.GetTeamWithPlayers(ctx, f.conn, first.ID)
	if err != nil || updated.Name != "Renamed Wolves" || updated.Players[0].ID != first.Players[0].ID || updated.Players[1].ID != first.Players[1].ID {
		t.Fatalf("stale group form could not save team with stable player IDs: %+v, %v", updated, err)
	}
	body := f.get("/control_panel/teams")
	if strings.Contains(body, fmt.Sprintf(`action="/control_panel/teams/groups/%d/delete"`, a)) || !strings.Contains(body, `data-team-name="Renamed Wolves"`) {
		t.Fatal("overview must render the surviving team without the deleted group")
	}
	// A stale create form is safe as well.
	late := f.team("Late arrival", a)
	f.membership(late.ID, 0)
	for _, id := range []int64{b, unused} {
		f.post(fmt.Sprintf("/control_panel/teams/groups/%d/delete", id), nil, false)
	}
	f.membership(second.ID, 0)
	if body := f.get("/control_panel/teams"); !strings.Contains(body, `data-team-name="Dragons"`) || !strings.Contains(body, `data-team-name="Late arrival"`) {
		t.Fatal("deleting every group must leave all teams usable")
	}
}

func TestTeamGroupsHTTPRejectsInvalidAssignmentWithoutMutation(t *testing.T) {
	f := newTeamGroupsFixture(t)
	ctx := context.Background()
	groupID := f.group("A")
	team := f.team("Wolves", groupID)
	for _, input := range []string{"0", "-1", "invalid", "1.5", "1e2", "9223372036854775808"} {
		t.Run(input, func(t *testing.T) {
			form := groupTeamForm("Must not be saved")
			form.Set("group_id", input)
			f.post("/control_panel/teams/create", form, true)
			f.post(fmt.Sprintf("/control_panel/teams/%d/update", team.ID), form, true)
			teams, err := db.ListTeamsWithPlayers(ctx, f.conn)
			if err != nil || len(teams) != 1 || !reflect.DeepEqual(teams[0], team) {
				t.Fatalf("invalid group input mutated a team or its players: %+v, %v", teams, err)
			}
			f.membership(team.ID, groupID)
		})
	}
	// Older clients can omit the field; an absent optional value means no group.
	f.post(fmt.Sprintf("/control_panel/teams/%d/update", team.ID), groupTeamForm(team.Name), false)
	f.membership(team.ID, 0)
	f.post("/control_panel/teams/groups/create", url.Values{"group_name": {"   "}}, true)
	groups, err := db.ListTeamGroups(ctx, f.conn)
	if err != nil || len(groups) != 1 || groups[0].ID != groupID {
		t.Fatalf("invalid group creation changed groups: %+v, %v", groups, err)
	}
}

func TestTeamGroupsDoNotAffectMatchSelectionOrActiveMatch(t *testing.T) {
	f := newTeamGroupsFixture(t)
	ctx := context.Background()
	teams := []db.TeamWithPlayers{f.team("Wolves", 0), f.team("Dragons", 0), f.team("Bears", 0), f.team("Foxes", 0)}
	beforeSelection := f.get("/control_panel")
	a, b := f.group("A"), f.group("B")
	f.update(teams[0], fmt.Sprint(a))
	f.update(teams[1], fmt.Sprint(a))
	f.update(teams[2], fmt.Sprint(b))
	if body := f.get("/control_panel"); body != beforeSelection {
		t.Fatal("adding and assigning visual groups changed match selection HTML or team ordering")
	}
	for _, pair := range [][2]int{{0, 1}, {0, 2}, {0, 3}, {3, 2}} {
		f.post("/control_panel/start_match", url.Values{"team1": {fmt.Sprint(teams[pair[0]].ID)}, "team2": {fmt.Sprint(teams[pair[1]].ID)}}, false)
		current := f.active.Get()
		if current == nil || current.Team1.TeamID != fmt.Sprint(teams[pair[0]].ID) || current.Team2.TeamID != fmt.Sprint(teams[pair[1]].ID) {
			t.Fatalf("groups restricted the requested team pair %v: %+v", pair, current)
		}
	}
	f.active.Inc(fmt.Sprint(teams[3].Players[0].ID))
	f.active.Inc(fmt.Sprint(teams[2].Players[1].ID))
	f.active.ToggleDisplaySwap()
	f.active.TimerSet(time.Unix(300, 0), 195000)
	beforeActive := f.active.Get()
	beforeSnapshot := buildSnapshot(beforeActive)
	beforeSnapshot.TimerServerMs = 0 // The response timestamp naturally changes between reads.
	withoutClock := func(body string) string {
		return regexp.MustCompile(`data-server-ms="[0-9]+"`).ReplaceAllString(body, `data-server-ms=""`)
	}
	beforePanel := withoutClock(f.get("/control_panel/active_match"))
	beforeSelection = f.get("/control_panel")
	events := f.hub.Subscribe()
	defer f.hub.Unsubscribe(events)
	c := f.group("C")
	f.update(teams[3], fmt.Sprint(c))
	f.update(teams[2], fmt.Sprint(a))
	f.update(teams[0], "")
	for _, id := range []int64{a, b, c} {
		f.post(fmt.Sprintf("/control_panel/teams/groups/%d/delete", id), nil, false)
	}
	afterSnapshot := buildSnapshot(f.active.Get())
	afterSnapshot.TimerServerMs = 0
	if !reflect.DeepEqual(f.active.Get(), beforeActive) || !reflect.DeepEqual(afterSnapshot, beforeSnapshot) {
		t.Fatal("visual group operations changed active teams, players, goals, sides, timer, or audience snapshot")
	}
	if withoutClock(f.get("/control_panel/active_match")) != beforePanel || f.get("/control_panel") != beforeSelection {
		t.Fatal("visual group operations changed an active match or selection panel")
	}
	select {
	case event := <-events:
		t.Fatalf("visual group operation unexpectedly broadcast a score event: %s", event)
	default:
	}
	// Running countdown state must also remain untouched, including its original timestamp.
	f.active.TimerToggleRun(time.Now())
	beforeRunning := f.active.Get()
	d := f.group("D")
	f.update(teams[3], fmt.Sprint(d))
	f.post(fmt.Sprintf("/control_panel/teams/groups/%d/delete", d), nil, false)
	if !reflect.DeepEqual(f.active.Get(), beforeRunning) {
		t.Fatal("group creation, assignment, or deletion reset the running match timer")
	}
	settings, err := db.GetTimerSettings(ctx, f.conn)
	if err != nil || settings.DefaultMinutes != 5 || settings.DefaultSeconds != 0 || !settings.ShowByDefault {
		t.Fatalf("group operations changed default timer settings: %+v, %v", settings, err)
	}
}
