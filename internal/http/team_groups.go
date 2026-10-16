package http

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"score-table/internal/db"
)

// Group metadata is deliberately limited to the teams overview.
type teamOverviewView struct {
	teamView
	db.TeamRecord
	GroupID string
}

type teamGroupView struct {
	ID    string
	Name  string
	Teams []teamOverviewView
}

func optionalTeamGroupID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid group ID")
	}
	return id, nil
}

func redirectTeamsError(w http.ResponseWriter, r *http.Request, message string) {
	http.Redirect(w, r, "/control_panel/teams?err="+url.QueryEscape(message), http.StatusSeeOther)
}

func (h *Handlers) CreateTeamGroup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		redirectTeamsError(w, r, "invalid form")
		return
	}
	name := strings.TrimSpace(r.PostForm.Get("group_name"))
	if name == "" || utf8.RuneCountInString(name) > 80 {
		redirectTeamsError(w, r, "group name must contain 1 to 80 characters")
		return
	}
	if _, err := db.CreateTeamGroup(r.Context(), h.db, name); err != nil {
		redirectTeamsError(w, r, "failed to create group")
		return
	}
	http.Redirect(w, r, "/control_panel/teams", http.StatusSeeOther)
}

func (h *Handlers) DeleteTeamGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.ParseInt(chi.URLParam(r, "groupID"), 10, 64)
	if err != nil || groupID <= 0 {
		redirectTeamsError(w, r, "unknown group")
		return
	}
	if err := db.DeleteTeamGroup(r.Context(), h.db, groupID); err != nil {
		redirectTeamsError(w, r, "failed to delete group")
		return
	}
	http.Redirect(w, r, "/control_panel/teams", http.StatusSeeOther)
}
