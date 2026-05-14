package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"score-table/internal/db"
	"score-table/internal/match"
	"score-table/internal/views"
)

type HandlersDeps struct {
	DB       *sql.DB
	Renderer *views.Renderer
	Active   *match.Store
	Edit     *match.EditStore
	Hub      *ScoreHub
}

type Handlers struct {
	db       *sql.DB
	renderer *views.Renderer
	active   *match.Store
	edit     *match.EditStore
	hub      *ScoreHub
}

func NewHandlers(d HandlersDeps) *Handlers {
	return &Handlers{
		db:       d.DB,
		renderer: d.Renderer,
		active:   d.Active,
		edit:     d.Edit,
		hub:      d.Hub,
	}
}

type teamView struct {
	ID      string
	Name    string
	Players []playerView
}

type playerView struct {
	ID   string
	Name string
}

func toTeamViews(rows []db.TeamWithPlayers) []teamView {
	out := make([]teamView, 0, len(rows))
	for _, t := range rows {
		pv := make([]playerView, 0, len(t.Players))
		for _, p := range t.Players {
			pv = append(pv, playerView{ID: strconv.FormatInt(p.ID, 10), Name: p.Name})
		}
		out = append(out, teamView{
			ID:      strconv.FormatInt(t.ID, 10),
			Name:    t.Name,
			Players: pv,
		})
	}
	return out
}

func setNoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store, max-age=0")
}

func (h *Handlers) DisplayScore(w http.ResponseWriter, r *http.Request) {
	setNoStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "display_score.html", map[string]any{
		"HasActive": h.active.Get() != nil,
	})
}

func (h *Handlers) ScoreEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch := h.hub.Subscribe()
	defer h.hub.Unsubscribe(ch)

	// Send initial snapshot (or clear).
	if msg, ok := h.snapshotEvent(); ok {
		_, _ = w.Write(msg)
		flusher.Flush()
	} else {
		_, _ = w.Write(makeSSE("clear", []byte(`{}`)))
		flusher.Flush()
	}

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, _ = w.Write(msg)
			flusher.Flush()
		}
	}
}

func (h *Handlers) ControlPanel(w http.ResponseWriter, r *http.Request) {
	teamsRows, err := db.ListTeamsWithPlayers(r.Context(), h.db)
	if err != nil {
		http.Error(w, "failed to load teams", http.StatusInternalServerError)
		return
	}

	setNoStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	_ = h.renderer.Render(w, "control_panel.html", map[string]any{
		"Teams":       toTeamViews(teamsRows),
		"CanStart":    len(teamsRows) >= 2,
		"Error":       errMsg,
		"HasActive":   h.active.Get() != nil,
		"ActiveRoute": "/control_panel/active_match",
	})
}

func (h *Handlers) TeamsOverview(w http.ResponseWriter, r *http.Request) {
	teamsRows, err := db.ListTeamsWithPlayers(r.Context(), h.db)
	if err != nil {
		http.Error(w, "failed to load teams", http.StatusInternalServerError)
		return
	}

	setNoStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	_ = h.renderer.Render(w, "teams.html", map[string]any{
		"Teams": toTeamViews(teamsRows),
		"Error": errMsg,
	})
}

func (h *Handlers) CreateTeam(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/control_panel/teams?err="+url.QueryEscape("invalid form"), http.StatusSeeOther)
		return
	}
	teamName := strings.TrimSpace(r.FormValue("team_name"))
	p1 := strings.TrimSpace(r.FormValue("player1"))
	p2 := strings.TrimSpace(r.FormValue("player2"))
	if teamName == "" || p1 == "" || p2 == "" {
		http.Redirect(w, r, "/control_panel/teams?err="+url.QueryEscape("team name and 2 players are required"), http.StatusSeeOther)
		return
	}
	if _, err := db.CreateTeamWithTwoPlayers(r.Context(), h.db, teamName, p1, p2); err != nil {
		http.Redirect(w, r, "/control_panel/teams?err="+url.QueryEscape("failed to create team"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/control_panel/teams", http.StatusSeeOther)
}

func (h *Handlers) UpdateTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := strconv.ParseInt(chi.URLParam(r, "teamID"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/control_panel/teams?err="+url.QueryEscape("unknown team"), http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/control_panel/teams?err="+url.QueryEscape("invalid form"), http.StatusSeeOther)
		return
	}
	teamName := strings.TrimSpace(r.FormValue("team_name"))
	p1 := strings.TrimSpace(r.FormValue("player1"))
	p2 := strings.TrimSpace(r.FormValue("player2"))
	if teamName == "" || p1 == "" || p2 == "" {
		http.Redirect(w, r, "/control_panel/teams?err="+url.QueryEscape("team name and 2 players are required"), http.StatusSeeOther)
		return
	}
	if err := db.UpdateTeamAndTwoPlayers(r.Context(), h.db, teamID, teamName, p1, p2); err != nil {
		http.Redirect(w, r, "/control_panel/teams?err="+url.QueryEscape("failed to update team"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/control_panel/teams", http.StatusSeeOther)
}

func (h *Handlers) DeleteTeam(w http.ResponseWriter, r *http.Request) {
	teamID, err := strconv.ParseInt(chi.URLParam(r, "teamID"), 10, 64)
	if err == nil {
		_ = db.DeleteTeam(r.Context(), h.db, teamID)
	}
	http.Redirect(w, r, "/control_panel/teams", http.StatusSeeOther)
}

func (h *Handlers) Settings(w http.ResponseWriter, r *http.Request) {
	s, err := db.GetTimerSettings(r.Context(), h.db)
	if err != nil {
		s = db.TimerSettings{DefaultMinutes: 5, DefaultSeconds: 0, ShowByDefault: true}
	}
	setNoStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	_ = h.renderer.Render(w, "settings.html", map[string]any{
		"Error": errMsg,
		"Timer": map[string]any{
			"DefaultMinutes": s.DefaultMinutes,
			"DefaultSeconds": s.DefaultSeconds,
			"ShowByDefault":  s.ShowByDefault,
		},
	})
}

func (h *Handlers) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/control_panel/settings?err="+url.QueryEscape("invalid form"), http.StatusSeeOther)
		return
	}
	mins, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("default_minutes")))
	secs, _ := strconv.Atoi(strings.TrimSpace(r.FormValue("default_seconds")))
	show := strings.TrimSpace(r.FormValue("show_by_default")) == "1"
	if mins < 0 {
		mins = 0
	}
	if secs < 0 {
		secs = 0
	}
	if secs > 59 {
		secs = 59
	}
	if err := db.UpdateTimerSettings(r.Context(), h.db, db.TimerSettings{
		DefaultMinutes: mins,
		DefaultSeconds: secs,
		ShowByDefault:  show,
	}); err != nil {
		http.Redirect(w, r, "/control_panel/settings?err="+url.QueryEscape("failed to save settings"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/control_panel/settings", http.StatusSeeOther)
}

func (h *Handlers) StartMatch(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/control_panel?err=invalid+form", http.StatusSeeOther)
		return
	}

	team1ID := strings.TrimSpace(r.FormValue("team1"))
	team2ID := strings.TrimSpace(r.FormValue("team2"))
	if team1ID == "" || team2ID == "" {
		http.Redirect(w, r, "/control_panel?err=please+choose+two+teams", http.StatusSeeOther)
		return
	}
	if team1ID == team2ID {
		http.Redirect(w, r, "/control_panel?err=teams+must+be+different", http.StatusSeeOther)
		return
	}

	t1Int, err1 := strconv.ParseInt(team1ID, 10, 64)
	t2Int, err2 := strconv.ParseInt(team2ID, 10, 64)
	if err1 != nil || err2 != nil {
		http.Redirect(w, r, "/control_panel?err=unknown+team", http.StatusSeeOther)
		return
	}

	t1, err := db.GetTeamWithPlayers(r.Context(), h.db, t1Int)
	if err != nil {
		http.Redirect(w, r, "/control_panel?err=unknown+team", http.StatusSeeOther)
		return
	}
	t2, err := db.GetTeamWithPlayers(r.Context(), h.db, t2Int)
	if err != nil {
		http.Redirect(w, r, "/control_panel?err=unknown+team", http.StatusSeeOther)
		return
	}
	if len(t1.Players) == 0 || len(t2.Players) == 0 {
		http.Redirect(w, r, "/control_panel?err=teams+must+have+players", http.StatusSeeOther)
		return
	}

	now := time.Now()
	settings, err := db.GetTimerSettings(r.Context(), h.db)
	if err != nil {
		settings = db.TimerSettings{DefaultMinutes: 5, DefaultSeconds: 0, ShowByDefault: true}
	}
	if settings.DefaultMinutes < 0 {
		settings.DefaultMinutes = 0
	}
	if settings.DefaultSeconds < 0 {
		settings.DefaultSeconds = 0
	}
	if settings.DefaultSeconds > 59 {
		settings.DefaultSeconds = 59
	}
	defaultMs := int64(settings.DefaultMinutes*60+settings.DefaultSeconds) * 1000
	showTimer := settings.ShowByDefault && defaultMs > 0

	m := &match.ActiveMatch{
		Team1: match.TeamSide{
			TeamID:   strconv.FormatInt(t1.ID, 10),
			TeamName: t1.Name,
			Players:  toPlayerRefsDB(t1.Players),
		},
		Team2: match.TeamSide{
			TeamID:   strconv.FormatInt(t2.ID, 10),
			TeamName: t2.Name,
			Players:  toPlayerRefsDB(t2.Players),
		},
		StartedAt:       now,
		GoalsByPlayerID: map[string]int{},
		Timer: match.Timer{
			Show:        showTimer,
			DefaultMs:   defaultMs,
			RemainingMs: defaultMs,
			Running:     false,
			UpdatedAt:   now,
		},
	}

	h.active.Start(m)
	h.broadcastSnapshot()
	http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
}

func (h *Handlers) ActiveMatch(w http.ResponseWriter, r *http.Request) {
	m := h.active.Get()
	if m == nil {
		http.Redirect(w, r, "/control_panel", http.StatusSeeOther)
		return
	}

	setNoStore(w)
	now := time.Now()
	t1Score, t2Score := computeScores(m)
	goalsForView := make(map[string]int, len(m.Team1.Players)+len(m.Team2.Players))
	for _, p := range m.Team1.Players {
		goalsForView[p.PlayerID] = m.GoalsByPlayerID[p.PlayerID]
	}
	for _, p := range m.Team2.Players {
		goalsForView[p.PlayerID] = m.GoalsByPlayerID[p.PlayerID]
	}

	timerSnap, _ := h.active.TimerSnapshotNow(now)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "active_match.html", map[string]any{
		"Team1":   m.Team1,
		"Team2":   m.Team2,
		"Score1":  t1Score,
		"Score2":  t2Score,
		"Goals":   goalsForView,
		"Started": m.StartedAt.UTC().Format("2006-01-02 15:04:05"),
		"Display": map[string]any{
			"Swapped": m.DisplaySwap,
		},
		"Timer": map[string]any{
			"Supported":   m.Timer.DefaultMs > 0,
			"Show":        timerSnap.Show,
			"Running":     timerSnap.Running,
			"RemainingMs": timerSnap.RemainingMs,
			"DefaultMs":   timerSnap.DefaultMs,
			"ServerMs":    now.UnixMilli(),
			"Text":        formatMMSS(timerSnap.RemainingMs),
			"DefText":     formatMMSS(timerSnap.DefaultMs),
			"Min":         timerSnap.RemainingMs / 1000 / 60,
			"Sec":         (timerSnap.RemainingMs / 1000) % 60,
		},
	})
}

func (h *Handlers) SwapDisplaySides(w http.ResponseWriter, r *http.Request) {
	_ = h.active.ToggleDisplaySwap()
	h.broadcastSnapshot()
	http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
}

func (h *Handlers) TimerToggle(w http.ResponseWriter, r *http.Request) {
	_ = h.active.TimerToggleRun(time.Now())
	h.broadcastSnapshot()
	http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
}

func (h *Handlers) TimerReset(w http.ResponseWriter, r *http.Request) {
	_ = h.active.TimerReset(time.Now())
	h.broadcastSnapshot()
	http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
}

func (h *Handlers) TimerSet(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
		return
	}
	mins, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("minutes")), 10, 64)
	secs, _ := strconv.ParseInt(strings.TrimSpace(r.FormValue("seconds")), 10, 64)
	if mins < 0 {
		mins = 0
	}
	if secs < 0 {
		secs = 0
	}
	if secs > 59 {
		secs = 59
	}
	remainingMs := (mins*60 + secs) * 1000
	_ = h.active.TimerSet(time.Now(), remainingMs)
	h.broadcastSnapshot()
	http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
}

func (h *Handlers) TimerVisibility(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
		return
	}
	show := strings.TrimSpace(r.FormValue("show")) == "1"
	_ = h.active.TimerSetVisibility(time.Now(), show)
	h.broadcastSnapshot()
	http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
}

func (h *Handlers) PlayerInc(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerID")
	if !h.playerInActiveMatch(playerID) {
		http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
		return
	}
	_ = h.active.Inc(playerID)
	h.broadcastSnapshot()
	http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
}

func (h *Handlers) PlayerDec(w http.ResponseWriter, r *http.Request) {
	playerID := chi.URLParam(r, "playerID")
	if !h.playerInActiveMatch(playerID) {
		http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
		return
	}
	_ = h.active.Dec(playerID)
	h.broadcastSnapshot()
	http.Redirect(w, r, "/control_panel/active_match", http.StatusSeeOther)
}

func (h *Handlers) SaveMatch(w http.ResponseWriter, r *http.Request) {
	m := h.active.Get()
	if m == nil {
		http.Redirect(w, r, "/control_panel", http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	endedAt := time.Now()
	t1Score, t2Score := computeScores(m)

	matchID, err := db.InsertMatch(ctx, h.db, db.InsertMatchParams{
		Team1ID:    m.Team1.TeamID,
		Team1Name:  m.Team1.TeamName,
		Team2ID:    m.Team2.TeamID,
		Team2Name:  m.Team2.TeamName,
		Team1Score: t1Score,
		Team2Score: t2Score,
		StartedAt:  m.StartedAt,
		EndedAt:    endedAt,
	})
	if err != nil {
		http.Redirect(w, r, "/control_panel/active_match?err=save+failed", http.StatusSeeOther)
		return
	}

	rows := buildGoalRows(m)
	if err := db.InsertMatchPlayerGoals(ctx, h.db, matchID, rows); err != nil {
		_ = db.DeleteMatch(ctx, h.db, matchID)
		http.Redirect(w, r, "/control_panel/active_match?err=save+failed", http.StatusSeeOther)
		return
	}

	h.active.Discard()
	h.broadcastClear()
	http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
}

func (h *Handlers) DiscardMatch(w http.ResponseWriter, r *http.Request) {
	h.active.Discard()
	h.broadcastClear()
	http.Redirect(w, r, "/control_panel", http.StatusSeeOther)
}

func (h *Handlers) DiscardMatchBeacon(w http.ResponseWriter, r *http.Request) {
	// Best effort discard on browser back/close. Do not redirect.
	h.active.Discard()
	h.broadcastClear()
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) History(w http.ResponseWriter, r *http.Request) {
	matches, err := db.ListMatches(r.Context(), h.db)
	if err != nil {
		http.Error(w, "failed to load history", http.StatusInternalServerError)
		return
	}

	setNoStore(w)
	type matchView struct {
		db.MatchRow
		StartedAt string
		EndedAt   string
	}
	views := make([]matchView, 0, len(matches))
	for _, m := range matches {
		views = append(views, matchView{
			MatchRow:  m,
			StartedAt: formatMatchTime(m.StartedAt),
			EndedAt:   formatMatchTime(m.EndedAt),
		})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "history.html", map[string]any{
		"Matches": views,
	})
}

func formatMatchTime(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC().Format("2006-01-02 15:04:05")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format("2006-01-02 15:04:05")
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.UTC); err == nil {
		return t.UTC().Format("2006-01-02 15:04:05")
	}
	return s
}

func (h *Handlers) DeleteMatch(w http.ResponseWriter, r *http.Request) {
	matchIDStr := chi.URLParam(r, "matchID")
	id, err := strconv.ParseInt(matchIDStr, 10, 64)
	if err == nil {
		_ = db.DeleteMatch(r.Context(), h.db, id)
	}
	http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
}

func (h *Handlers) EditMatch(w http.ResponseWriter, r *http.Request) {
	matchID, err := strconv.ParseInt(chi.URLParam(r, "matchID"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
		return
	}

	mRowForView, err := db.GetMatch(r.Context(), h.db, matchID)
	if err != nil {
		http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
		return
	}

	// Start/edit session if needed.
	current := h.edit.Get()
	if current == nil || current.MatchID != matchID {
		goalRows, err := db.ListMatchPlayerGoals(r.Context(), h.db, matchID)
		if err != nil {
			http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
			return
		}

		team1Players := h.playersForTeam(mRowForView.Team1ID, goalRows, mRowForView.Team1ID)
		team2Players := h.playersForTeam(mRowForView.Team2ID, goalRows, mRowForView.Team2ID)

		goals := map[string]int{}
		for _, p := range team1Players {
			goals[p.PlayerID] = 0
		}
		for _, p := range team2Players {
			goals[p.PlayerID] = 0
		}
		for _, gr := range goalRows {
			goals[gr.PlayerID] += gr.Goals
		}

		h.edit.Start(&match.EditMatch{
			MatchID: matchID,
			Team1: match.TeamSide{
				TeamID:   mRowForView.Team1ID,
				TeamName: mRowForView.Team1Name,
				Players:  team1Players,
			},
			Team2: match.TeamSide{
				TeamID:   mRowForView.Team2ID,
				TeamName: mRowForView.Team2Name,
				Players:  team2Players,
			},
			GoalsByPlayerID: goals,
		})
	}

	e := h.edit.Get()
	if e == nil || e.MatchID != matchID {
		http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
		return
	}

	tmp := &match.ActiveMatch{Team1: e.Team1, Team2: e.Team2, GoalsByPlayerID: e.GoalsByPlayerID}
	s1, s2 := computeScores(tmp)
	goalsForView := make(map[string]int, len(e.Team1.Players)+len(e.Team2.Players))
	for _, p := range e.Team1.Players {
		goalsForView[p.PlayerID] = e.GoalsByPlayerID[p.PlayerID]
	}
	for _, p := range e.Team2.Players {
		goalsForView[p.PlayerID] = e.GoalsByPlayerID[p.PlayerID]
	}

	setNoStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "edit_match.html", map[string]any{
		"MatchID": matchID,
		"Team1":   e.Team1,
		"Team2":   e.Team2,
		"Score1":  s1,
		"Score2":  s2,
		"Goals":   goalsForView,
		"Ended":   formatMatchTime(mRowForView.EndedAt),
	})
}

func (h *Handlers) EditPlayerInc(w http.ResponseWriter, r *http.Request) {
	matchID, err := strconv.ParseInt(chi.URLParam(r, "matchID"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
		return
	}
	playerID := chi.URLParam(r, "playerID")
	_ = h.edit.Inc(matchID, playerID)
	http.Redirect(w, r, fmt.Sprintf("/control_panel/history/%d/edit", matchID), http.StatusSeeOther)
}

func (h *Handlers) EditPlayerDec(w http.ResponseWriter, r *http.Request) {
	matchID, err := strconv.ParseInt(chi.URLParam(r, "matchID"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
		return
	}
	playerID := chi.URLParam(r, "playerID")
	_ = h.edit.Dec(matchID, playerID)
	http.Redirect(w, r, fmt.Sprintf("/control_panel/history/%d/edit", matchID), http.StatusSeeOther)
}

func (h *Handlers) SaveMatchEdits(w http.ResponseWriter, r *http.Request) {
	matchID, err := strconv.ParseInt(chi.URLParam(r, "matchID"), 10, 64)
	if err != nil {
		http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
		return
	}
	e := h.edit.Get()
	if e == nil || e.MatchID != matchID {
		http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
		return
	}

	tmp := &match.ActiveMatch{Team1: e.Team1, Team2: e.Team2, GoalsByPlayerID: e.GoalsByPlayerID}
	s1, s2 := computeScores(tmp)
	rows := buildGoalRows(tmp)

	if err := db.UpdateMatchScoresAndGoals(r.Context(), h.db, matchID, s1, s2, rows); err != nil {
		http.Redirect(w, r, fmt.Sprintf("/control_panel/history/%d/edit", matchID), http.StatusSeeOther)
		return
	}

	h.edit.Discard()
	http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
}

func (h *Handlers) DiscardMatchEdits(w http.ResponseWriter, r *http.Request) {
	matchID, _ := strconv.ParseInt(chi.URLParam(r, "matchID"), 10, 64)
	e := h.edit.Get()
	if e != nil && e.MatchID == matchID {
		h.edit.Discard()
	}
	http.Redirect(w, r, "/control_panel/history", http.StatusSeeOther)
}

func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	stats, totals, err := db.PlayerStats(r.Context(), h.db)
	if err != nil {
		http.Error(w, "failed to load stats", http.StatusInternalServerError)
		return
	}

	teamsRows, err := db.ListTeamsWithPlayers(r.Context(), h.db)
	if err != nil {
		http.Error(w, "failed to load teams", http.StatusInternalServerError)
		return
	}

	setNoStore(w)
	type col struct {
		ID   string
		Name string
	}
	cols := make([]col, 0, len(teamsRows))
	for _, t := range teamsRows {
		cols = append(cols, col{ID: strconv.FormatInt(t.ID, 10), Name: t.Name})
	}

	type row struct {
		PlayerID   string
		PlayerName string
		ByOpp      map[string]int
		Total      int
	}
	rows := make([]row, 0, len(teamsRows)*2)
	for _, t := range teamsRows {
		for _, p := range t.Players {
			pid := strconv.FormatInt(p.ID, 10)
			byOpp := stats[pid]
			if byOpp == nil {
				byOpp = map[string]int{}
			}
			rows = append(rows, row{
				PlayerID:   pid,
				PlayerName: p.Name,
				ByOpp:      byOpp,
				Total:      totals[pid],
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Total != rows[j].Total {
			return rows[i].Total > rows[j].Total
		}
		return strings.ToLower(rows[i].PlayerName) < strings.ToLower(rows[j].PlayerName)
	})

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "stats.html", map[string]any{
		"Cols": cols,
		"Rows": rows,
	})
}

func (h *Handlers) snapshotEvent() ([]byte, bool) {
	m := h.active.Get()
	if m == nil {
		return nil, false
	}

	snap := buildSnapshot(m)
	b, _ := json.Marshal(snap)
	return makeSSE("snapshot", b), true
}

func (h *Handlers) broadcastSnapshot() {
	if msg, ok := h.snapshotEvent(); ok {
		h.hub.Broadcast(msg)
	}
}

func (h *Handlers) broadcastClear() {
	h.hub.Broadcast(makeSSE("clear", []byte(`{}`)))
}

type snapshot struct {
	Active bool `json:"active"`

	Team1Name  string `json:"team1Name"`
	Team2Name  string `json:"team2Name"`
	Team1Score int    `json:"team1Score"`
	Team2Score int    `json:"team2Score"`

	ShowTimer        bool  `json:"showTimer"`
	TimerRunning     bool  `json:"timerRunning"`
	TimerRemainingMs int64 `json:"timerRemainingMs"`
	TimerServerMs    int64 `json:"timerServerMs"`
}

func buildSnapshot(m *match.ActiveMatch) snapshot {
	s1, s2 := computeScores(m)
	now := time.Now()
	rem := int64(0)
	if m.Timer.Show && m.Timer.DefaultMs > 0 {
		rem = m.Timer.RemainingMs
		if m.Timer.Running {
			elapsed := now.Sub(m.Timer.UpdatedAt).Milliseconds()
			rem = m.Timer.RemainingMs - elapsed
			if rem < 0 {
				rem = 0
			}
		}
	}
	t1Name := m.Team1.TeamName
	t2Name := m.Team2.TeamName
	t1Score := s1
	t2Score := s2
	if m.DisplaySwap {
		t1Name, t2Name = t2Name, t1Name
		t1Score, t2Score = t2Score, t1Score
	}

	return snapshot{
		Active:           true,
		Team1Name:        t1Name,
		Team2Name:        t2Name,
		Team1Score:       t1Score,
		Team2Score:       t2Score,
		ShowTimer:        m.Timer.Show && m.Timer.DefaultMs > 0,
		TimerRunning:     m.Timer.Show && m.Timer.Running,
		TimerRemainingMs: rem,
		TimerServerMs:    now.UnixMilli(),
	}
}

func formatMMSS(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	total := ms / 1000
	min := total / 60
	sec := total % 60
	return fmt.Sprintf("%02d:%02d", min, sec)
}

func computeScores(m *match.ActiveMatch) (t1, t2 int) {
	isT1 := map[string]struct{}{}
	isT2 := map[string]struct{}{}
	for _, p := range m.Team1.Players {
		isT1[p.PlayerID] = struct{}{}
	}
	for _, p := range m.Team2.Players {
		isT2[p.PlayerID] = struct{}{}
	}
	for pid, goals := range m.GoalsByPlayerID {
		if goals <= 0 {
			continue
		}
		if _, ok := isT1[pid]; ok {
			t1 += goals
		} else if _, ok := isT2[pid]; ok {
			t2 += goals
		}
	}
	return t1, t2
}

func (h *Handlers) playerInActiveMatch(playerID string) bool {
	m := h.active.Get()
	if m == nil {
		return false
	}
	for _, p := range m.Team1.Players {
		if p.PlayerID == playerID {
			return true
		}
	}
	for _, p := range m.Team2.Players {
		if p.PlayerID == playerID {
			return true
		}
	}
	return false
}

func toPlayerRefsDB(players []db.PlayerRow) []match.PlayerRef {
	out := make([]match.PlayerRef, 0, len(players))
	for _, p := range players {
		out = append(out, match.PlayerRef{
			PlayerID:   strconv.FormatInt(p.ID, 10),
			PlayerName: p.Name,
		})
	}
	return out
}

func (h *Handlers) playersForTeam(teamID string, goalRows []db.PlayerGoalRow, rowTeamID string) []match.PlayerRef {
	if id, err := strconv.ParseInt(strings.TrimSpace(teamID), 10, 64); err == nil {
		if t, err := db.GetTeamWithPlayers(context.Background(), h.db, id); err == nil && len(t.Players) > 0 {
			return toPlayerRefsDB(t.Players)
		}
	}
	seen := map[string]match.PlayerRef{}
	for _, r := range goalRows {
		if r.ScoringTeamID != rowTeamID {
			continue
		}
		seen[r.PlayerID] = match.PlayerRef{PlayerID: r.PlayerID, PlayerName: r.PlayerName}
	}
	out := make([]match.PlayerRef, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].PlayerName) < strings.ToLower(out[j].PlayerName) })
	return out
}

type goalRow struct {
	PlayerID         string
	PlayerName       string
	ScoringTeamID    string
	ScoringTeamName  string
	OpponentTeamID   string
	OpponentTeamName string
	Goals            int
}

func buildGoalRows(m *match.ActiveMatch) []db.PlayerGoalRow {
	isT1 := map[string]match.PlayerRef{}
	isT2 := map[string]match.PlayerRef{}
	for _, p := range m.Team1.Players {
		isT1[p.PlayerID] = p
	}
	for _, p := range m.Team2.Players {
		isT2[p.PlayerID] = p
	}

	var rows []db.PlayerGoalRow
	for pid, goals := range m.GoalsByPlayerID {
		if goals <= 0 {
			continue
		}
		if p, ok := isT1[pid]; ok {
			rows = append(rows, db.PlayerGoalRow{
				PlayerID:         p.PlayerID,
				PlayerName:       p.PlayerName,
				ScoringTeamID:    m.Team1.TeamID,
				ScoringTeamName:  m.Team1.TeamName,
				OpponentTeamID:   m.Team2.TeamID,
				OpponentTeamName: m.Team2.TeamName,
				Goals:            goals,
			})
		} else if p, ok := isT2[pid]; ok {
			rows = append(rows, db.PlayerGoalRow{
				PlayerID:         p.PlayerID,
				PlayerName:       p.PlayerName,
				ScoringTeamID:    m.Team2.TeamID,
				ScoringTeamName:  m.Team2.TeamName,
				OpponentTeamID:   m.Team1.TeamID,
				OpponentTeamName: m.Team1.TeamName,
				Goals:            goals,
			})
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].ScoringTeamID != rows[j].ScoringTeamID {
			return rows[i].ScoringTeamID < rows[j].ScoringTeamID
		}
		return rows[i].PlayerID < rows[j].PlayerID
	})

	return rows
}

func makeSSE(event string, data []byte) []byte {
	// NOTE: data must be one line for simplest parsing on client.
	data = []byte(strings.ReplaceAll(string(data), "\n", ""))
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event, data))
}

var errNoMatch = errors.New("no active match")

func withActiveMatch(ctx context.Context, s *match.Store, fn func(m *match.ActiveMatch) error) error {
	m := s.Get()
	if m == nil {
		return errNoMatch
	}
	return fn(m)
}
