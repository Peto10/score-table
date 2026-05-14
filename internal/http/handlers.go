package http

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"floorball-score-table/internal/config"
	"floorball-score-table/internal/db"
	"floorball-score-table/internal/match"
	"floorball-score-table/internal/views"
)

type HandlersDeps struct {
	Config   *config.Config
	DB       *sql.DB
	Renderer *views.Renderer
	Active   *match.Store
	Edit     *match.EditStore
	Hub      *ScoreHub
}

type Handlers struct {
	cfg      *config.Config
	cfgIndex config.Index
	db       *sql.DB
	renderer *views.Renderer
	active   *match.Store
	edit     *match.EditStore
	hub      *ScoreHub
}

func NewHandlers(d HandlersDeps) *Handlers {
	return &Handlers{
		cfg:      d.Config,
		cfgIndex: d.Config.BuildIndex(),
		db:       d.DB,
		renderer: d.Renderer,
		active:   d.Active,
		edit:     d.Edit,
		hub:      d.Hub,
	}
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
	setNoStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	errMsg := strings.TrimSpace(r.URL.Query().Get("err"))
	_ = h.renderer.Render(w, "control_panel.html", map[string]any{
		"Teams":       h.cfg.Teams,
		"Error":       errMsg,
		"HasActive":   h.active.Get() != nil,
		"ActiveRoute": "/control_panel/active_match",
	})
}

func (h *Handlers) TeamsOverview(w http.ResponseWriter, r *http.Request) {
	setNoStore(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "teams.html", map[string]any{
		"Teams": h.cfg.Teams,
	})
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

	t1, ok1 := h.cfgIndex.TeamsByID[team1ID]
	t2, ok2 := h.cfgIndex.TeamsByID[team2ID]
	if !ok1 || !ok2 {
		http.Redirect(w, r, "/control_panel?err=unknown+team", http.StatusSeeOther)
		return
	}

	m := &match.ActiveMatch{
		Team1: match.TeamSide{
			TeamID:   t1.ID,
			TeamName: t1.Name,
			Players:  toPlayerRefs(t1.Players),
		},
		Team2: match.TeamSide{
			TeamID:   t2.ID,
			TeamName: t2.Name,
			Players:  toPlayerRefs(t2.Players),
		},
		StartedAt:       time.Now(),
		GoalsByPlayerID: map[string]int{},
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
	t1Score, t2Score := computeScores(m)
	goalsForView := make(map[string]int, len(m.Team1.Players)+len(m.Team2.Players))
	for _, p := range m.Team1.Players {
		goalsForView[p.PlayerID] = m.GoalsByPlayerID[p.PlayerID]
	}
	for _, p := range m.Team2.Players {
		goalsForView[p.PlayerID] = m.GoalsByPlayerID[p.PlayerID]
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = h.renderer.Render(w, "active_match.html", map[string]any{
		"Team1":   m.Team1,
		"Team2":   m.Team2,
		"Score1":  t1Score,
		"Score2":  t2Score,
		"Goals":   goalsForView,
		"Started": m.StartedAt.UTC().Format("2006-01-02 15:04:05"),
	})
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

	setNoStore(w)
	type col struct {
		ID   string
		Name string
	}
	cols := make([]col, 0, len(h.cfg.Teams))
	for _, t := range h.cfg.Teams {
		cols = append(cols, col{ID: t.ID, Name: t.Name})
	}

	type row struct {
		PlayerID   string
		PlayerName string
		ByOpp      map[string]int
		Total      int
	}
	rows := make([]row, 0, len(h.cfgIndex.PlayersByID))
	for pid, v := range h.cfgIndex.PlayersByID {
		byOpp := stats[pid]
		if byOpp == nil {
			byOpp = map[string]int{}
		}
		rows = append(rows, row{
			PlayerID:   pid,
			PlayerName: v.Player.Name,
			ByOpp:      byOpp,
			Total:      totals[pid],
		})
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
}

func buildSnapshot(m *match.ActiveMatch) snapshot {
	s1, s2 := computeScores(m)
	return snapshot{
		Active:     true,
		Team1Name:  m.Team1.TeamName,
		Team2Name:  m.Team2.TeamName,
		Team1Score: s1,
		Team2Score: s2,
	}
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

func toPlayerRefs(players []config.Player) []match.PlayerRef {
	out := make([]match.PlayerRef, 0, len(players))
	for _, p := range players {
		out = append(out, match.PlayerRef{PlayerID: p.ID, PlayerName: p.Name})
	}
	return out
}

func (h *Handlers) playersForTeam(teamID string, goalRows []db.PlayerGoalRow, rowTeamID string) []match.PlayerRef {
	if t, ok := h.cfgIndex.TeamsByID[teamID]; ok && len(t.Players) > 0 {
		return toPlayerRefs(t.Players)
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
