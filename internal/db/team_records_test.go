package db

import (
	"context"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestListTeamRecordsSavedMatchLifecycle(t *testing.T) {
	sqlDB, err := OpenAndInit(filepath.Join(t.TempDir(), "records.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx := context.Background()
	var teamIDs [3]string
	var numericIDs [3]int64
	for i, name := range []string{"Same name", "Same name", "No games"} {
		id, err := CreateTeamWithTwoPlayers(ctx, sqlDB, name, "Player one", "Player two")
		if err != nil {
			t.Fatal(err)
		}
		numericIDs[i], teamIDs[i] = id, strconv.FormatInt(id, 10)
	}
	if records, err := ListTeamRecords(ctx, sqlDB); err != nil || len(records) != 0 || records[teamIDs[2]] != (TeamRecord{}) {
		t.Fatalf("teams without saved matches must have zero records: %v, err=%v", records, err)
	}
	insert := func(left, right, leftScore, rightScore int) int64 {
		t.Helper()
		id, err := InsertMatch(ctx, sqlDB, InsertMatchParams{
			Team1ID: teamIDs[left], Team1Name: "Same name", Team1Score: leftScore,
			Team2ID: teamIDs[right], Team2Name: "Same name", Team2Score: rightScore,
			StartedAt: time.Unix(100, 0), EndedAt: time.Unix(200, 0),
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	check := func(wantA, wantB TeamRecord) {
		t.Helper()
		records, err := ListTeamRecords(ctx, sqlDB)
		if err != nil {
			t.Fatal(err)
		}
		if len(records) != 2 || records[teamIDs[0]] != wantA || records[teamIDs[1]] != wantB || records[teamIDs[2]] != (TeamRecord{}) {
			t.Fatalf("records = %v; want %s=%+v, %s=%+v, no-games team zero", records, teamIDs[0], wantA, teamIDs[1], wantB)
		}
	}
	first := insert(0, 1, 3, 1)
	second := insert(0, 1, 2, 4)
	insert(1, 0, 0, 2)
	insert(1, 0, 0, 0)
	check(TeamRecord{Wins: 2, Losses: 1, Draws: 1}, TeamRecord{Wins: 1, Losses: 2, Draws: 1})
	// Renaming a team preserves its ID-based record and historical match names.
	if err := UpdateTeamAndTwoPlayers(ctx, sqlDB, numericIDs[0], "Renamed team", "Player one", "Player two"); err != nil {
		t.Fatal(err)
	}
	check(TeamRecord{Wins: 2, Losses: 1, Draws: 1}, TeamRecord{Wins: 1, Losses: 2, Draws: 1})
	if err := UpdateMatchScores(ctx, sqlDB, first, 1, 3); err != nil {
		t.Fatal(err)
	}
	check(TeamRecord{Wins: 1, Losses: 2, Draws: 1}, TeamRecord{Wins: 2, Losses: 1, Draws: 1})
	if err := DeleteMatch(ctx, sqlDB, second); err != nil {
		t.Fatal(err)
	}
	check(TeamRecord{Wins: 1, Losses: 1, Draws: 1}, TeamRecord{Wins: 1, Losses: 1, Draws: 1})
}
