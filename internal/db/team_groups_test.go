package db

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"
)

func openTeamGroupTestDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "groups.sqlite")
	sqlDB, err := OpenAndInit(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB, path
}

func TestTeamGroupsLifecycleAndPersistence(t *testing.T) {
	sqlDB, path := openTeamGroupTestDB(t)
	ctx := context.Background()
	checkMemberships := func(want map[int64]int64) {
		t.Helper()
		got, err := ListTeamGroupMemberships(ctx, sqlDB)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("memberships = %v, err=%v; want %v", got, err, want)
		}
	}
	if groups, err := ListTeamGroups(ctx, sqlDB); err != nil || len(groups) != 0 {
		t.Fatalf("new database must have no groups: %v, err=%v", groups, err)
	}
	a, err := CreateTeamWithTwoPlayers(ctx, sqlDB, "Team A", "A1", "A2")
	if err != nil {
		t.Fatal(err)
	}
	checkMemberships(map[int64]int64{})
	first, err := CreateTeamGroup(ctx, sqlDB, "Group Z")
	if err != nil {
		t.Fatal(err)
	}
	second, err := CreateTeamGroup(ctx, sqlDB, "Group A")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ListTeamGroups(ctx, sqlDB); err != nil || !reflect.DeepEqual(got, []TeamGroupRow{{ID: first, Name: "Group Z"}, {ID: second, Name: "Group A"}}) {
		t.Fatalf("groups must retain creation order: %v, err=%v", got, err)
	}
	b, err := CreateTeamWithTwoPlayersInGroup(ctx, sqlDB, "Team B", "B1", "B2", first)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := ListTeamsWithPlayers(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := InsertMatch(ctx, sqlDB, InsertMatchParams{
		Team1ID: strconv.FormatInt(a, 10), Team1Name: "Team A", Team1Score: 4,
		Team2ID: strconv.FormatInt(b, 10), Team2Name: "Team B", Team2Score: 2,
		StartedAt: time.Unix(100, 0), EndedAt: time.Unix(200, 0),
	}); err != nil {
		t.Fatal(err)
	}
	wantRecords, err := ListTeamRecords(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	// Legacy edits preserve membership, while grouped edits can move or remove it.
	if err := UpdateTeamAndTwoPlayers(ctx, sqlDB, b, "Team B", "B1", "B2"); err != nil {
		t.Fatal(err)
	}
	for _, groupID := range []int64{first, second, 0, first} {
		if err := UpdateTeamAndTwoPlayersInGroup(ctx, sqlDB, a, "Team A", "A1", "A2", groupID); err != nil {
			t.Fatal(err)
		}
		want := map[int64]int64{b: first}
		if groupID != 0 {
			want[a] = groupID
		}
		checkMemberships(want)
	}
	for _, groupID := range []int64{first, second} { // Occupied, then empty.
		if err := DeleteTeamGroup(ctx, sqlDB, groupID); err != nil {
			t.Fatal(err)
		}
	}
	checkMemberships(map[int64]int64{})
	if got, err := ListTeamsWithPlayers(ctx, sqlDB); err != nil || !reflect.DeepEqual(got, baseline) {
		t.Fatalf("group operations changed match selection or player IDs: %v, err=%v", got, err)
	}
	if err := UpdateTeamAndTwoPlayersInGroup(ctx, sqlDB, a, "Renamed A", "Renamed A1", "A2", first); err != nil {
		t.Fatal(err)
	}
	if got, err := GetTeamWithPlayers(ctx, sqlDB, a); err != nil || got.Name != "Renamed A" || got.Players[0].Name != "Renamed A1" || got.Players[0].ID != baseline[0].Players[0].ID || got.Players[1].ID != baseline[0].Players[1].ID {
		t.Fatalf("stale group edit must save names with stable player IDs: %+v, err=%v", got, err)
	}
	if _, err := CreateTeamWithTwoPlayersInGroup(ctx, sqlDB, "Stale form team", "C1", "C2", first); err != nil {
		t.Fatal(err)
	}
	checkMemberships(map[int64]int64{})
	persisted, err := CreateTeamGroup(ctx, sqlDB, "Persisted group")
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateTeamAndTwoPlayersInGroup(ctx, sqlDB, a, "Renamed A", "Renamed A1", "A2", persisted); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	sqlDB, err = OpenAndInit(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	checkMemberships(map[int64]int64{a: persisted})
	if got, err := ListTeamGroups(ctx, sqlDB); err != nil || !reflect.DeepEqual(got, []TeamGroupRow{{ID: persisted, Name: "Persisted group"}}) {
		t.Fatalf("groups did not survive reopening: %v, err=%v", got, err)
	}
	if got, err := ListTeamRecords(ctx, sqlDB); err != nil || !reflect.DeepEqual(got, wantRecords) {
		t.Fatalf("group changes affected saved results: %v, err=%v", got, err)
	}
}

func TestTeamGroupsUpgradeLegacyDatabase(t *testing.T) {
	sqlDB, path := openTeamGroupTestDB(t)
	ctx := context.Background()
	legacyTeam, err := CreateTeamWithTwoPlayers(ctx, sqlDB, "Legacy team", "One", "Two")
	if err != nil {
		t.Fatal(err)
	}
	matchID, err := InsertMatch(ctx, sqlDB, InsertMatchParams{
		Team1ID: strconv.FormatInt(legacyTeam, 10), Team1Name: "Legacy team", Team1Score: 5,
		Team2ID: "archived-opponent", Team2Name: "Former team", Team2Score: 2,
		StartedAt: time.Unix(100, 0), EndedAt: time.Unix(200, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	beforeMatch, err := GetMatch(ctx, sqlDB, matchID)
	if err != nil {
		t.Fatal(err)
	}
	before, err := ListTeamsWithPlayers(ctx, sqlDB)
	if err != nil {
		t.Fatal(err)
	}
	settings := TimerSettings{DefaultMinutes: 3, DefaultSeconds: 15, ShowByDefault: false}
	if err := UpdateTimerSettings(ctx, sqlDB, settings); err != nil {
		t.Fatal(err)
	}
	// Removing only the new tables recreates the pre-group schema with existing data.
	if _, err := sqlDB.Exec(`DROP TABLE team_group_memberships; DROP TABLE team_groups;`); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		reopened, err := OpenAndInit(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = reopened.Close() })
		if got, err := ListTeamGroups(ctx, reopened); err != nil || len(got) != 0 {
			t.Fatalf("legacy upgrade must create empty groups idempotently: %v, err=%v", got, err)
		}
		if got, err := ListTeamGroupMemberships(ctx, reopened); err != nil || len(got) != 0 {
			t.Fatalf("legacy teams must remain ungrouped: %v, err=%v", got, err)
		}
		if got, err := ListTeamsWithPlayers(ctx, reopened); err != nil || !reflect.DeepEqual(got, before) {
			t.Fatalf("legacy selection data changed: %v, err=%v", got, err)
		}
		if got, err := GetTimerSettings(ctx, reopened); err != nil || got != settings {
			t.Fatalf("legacy settings changed: %+v, err=%v", got, err)
		}
		if got, err := GetMatch(ctx, reopened, matchID); err != nil || !reflect.DeepEqual(got, beforeMatch) {
			t.Fatalf("legacy match data changed: %+v, err=%v", got, err)
		}
		if err := reopened.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTeamGroupWritesWithoutForeignKeys(t *testing.T) {
	sqlDB, _ := openTeamGroupTestDB(t)
	sqlDB.SetMaxOpenConns(1)
	ctx := context.Background()
	if _, err := sqlDB.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	group, err := CreateTeamGroup(ctx, sqlDB, "Occupied")
	if err != nil {
		t.Fatal(err)
	}
	team, err := CreateTeamWithTwoPlayersInGroup(ctx, sqlDB, "Kept team", "One", "Two", group)
	if err != nil {
		t.Fatal(err)
	}
	before, err := GetTeamWithPlayers(ctx, sqlDB, team)
	if err != nil {
		t.Fatal(err)
	}
	deletedTeam, err := CreateTeamWithTwoPlayersInGroup(ctx, sqlDB, "Delete this team", "X", "Y", group)
	if err != nil {
		t.Fatal(err)
	}
	if err := DeleteTeam(ctx, sqlDB, deletedTeam); err != nil {
		t.Fatal(err)
	}
	var memberships int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM team_group_memberships WHERE team_id = ?`, deletedTeam).Scan(&memberships); err != nil || memberships != 0 {
		t.Fatalf("team deletion must remove its membership without FK enforcement: %d, err=%v", memberships, err)
	}
	if got, err := ListTeamGroups(ctx, sqlDB); err != nil || !reflect.DeepEqual(got, []TeamGroupRow{{ID: group, Name: "Occupied"}}) {
		t.Fatalf("team deletion removed its group: %v, err=%v", got, err)
	}
	if got, err := ListTeamGroupMemberships(ctx, sqlDB); err != nil || !reflect.DeepEqual(got, map[int64]int64{team: group}) {
		t.Fatalf("team deletion changed another membership: %v, err=%v", got, err)
	}
	for _, update := range []func() error{
		func() error { return UpdateTeamAndTwoPlayers(ctx, sqlDB, 999999, "Missing", "X", "Y") },
		func() error { return UpdateTeamAndTwoPlayersInGroup(ctx, sqlDB, 999999, "Missing", "X", "Y", group) },
	} {
		if err := update(); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("missing team update must fail before creating orphan players: %v", err)
		}
	}
	var orphans int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM players WHERE team_id = 999999`).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("missing team update created orphan players: %d, err=%v", orphans, err)
	}
	if _, err := sqlDB.Exec(`UPDATE team_group_memberships SET group_id = 999998 WHERE team_id = ?;
INSERT INTO team_group_memberships (team_id, group_id) VALUES (999999, ?);`, team, group); err != nil {
		t.Fatal(err)
	}
	if got, err := ListTeamGroupMemberships(ctx, sqlDB); err != nil || len(got) != 0 {
		t.Fatalf("stale membership references must be excluded: %v, err=%v", got, err)
	}
	if err := UpdateTeamAndTwoPlayersInGroup(ctx, sqlDB, team, "Kept team", "One", "Two", group); err != nil {
		t.Fatal(err)
	}
	if err := DeleteTeamGroup(ctx, sqlDB, group); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM team_group_memberships`).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("group deletion must explicitly remove memberships without FK enforcement: %d, err=%v", orphans, err)
	}
	group, err = CreateTeamGroup(ctx, sqlDB, "Atomic assignment")
	if err != nil {
		t.Fatal(err)
	}
	if err := UpdateTeamAndTwoPlayersInGroup(ctx, sqlDB, team, "Kept team", "One", "Two", group); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`CREATE TRIGGER reject_membership BEFORE INSERT ON team_group_memberships
BEGIN SELECT RAISE(ABORT, 'assignment rejected'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTeamWithTwoPlayersInGroup(ctx, sqlDB, "Rollback team", "X", "Y", group); err == nil {
		t.Fatal("team creation must roll back when assignment fails")
	}
	if err := UpdateTeamAndTwoPlayersInGroup(ctx, sqlDB, team, "Rollback name", "X", "Y", group); err == nil {
		t.Fatal("team edit must roll back when assignment fails")
	}
	if got, err := ListTeamsWithPlayers(ctx, sqlDB); err != nil || !reflect.DeepEqual(got, []TeamWithPlayers{before}) {
		t.Fatalf("failed assignment or group deletion changed teams/players: %+v, err=%v", got, err)
	}
	if got, err := ListTeamGroupMemberships(ctx, sqlDB); err != nil || !reflect.DeepEqual(got, map[int64]int64{team: group}) {
		t.Fatalf("failed assignment did not restore the previous membership: %v, err=%v", got, err)
	}
}
