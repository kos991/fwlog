package main

import (
	"strings"
	"testing"
	"time"
)

func TestBuildLogDateOptionsIncludesTodayWhenArchivedLogExists(t *testing.T) {
	today := time.Date(2026, 7, 1, 9, 30, 0, 0, time.Local)
	snapshots := []LogFileSnapshot{
		{Path: "/data/sangfor_fw_log/sangfor.log-20260630", Size: 128},
		{Path: "/data/sangfor_fw_log/sangfor.log-20260701", Size: 256},
	}

	options := buildLogDateOptions(snapshots, today)

	if len(options) != 2 {
		t.Fatalf("expected two date options, got %#v", options)
	}
	if options[0].Value != "date:2026-07-01" || !strings.Contains(options[0].Label, "今天") {
		t.Fatalf("today archive should be the first visible date option, got %#v", options[0])
	}
	if options[1].Value != "date:2026-06-30" {
		t.Fatalf("older archive should be sorted after today, got %#v", options[1])
	}
}

func TestGetTimeFilterSupportsExplicitLogDate(t *testing.T) {
	filter := getTimeFilter("date:2026-07-01")
	if !strings.Contains(filter, "strptime") || !strings.Contains(filter, "< CAST(? AS TIMESTAMP)") {
		t.Fatalf("explicit date filter should compare parsed log timestamp range, got %s", filter)
	}

	args := timeFilterArgs("date:2026-07-01")
	if len(args) != 4 ||
		args[0].(string) != "2026" ||
		args[1].(string) != "2026-07-01 00:00:00" ||
		args[2].(string) != "2026" ||
		args[3].(string) != "2026-07-02 00:00:00" {
		t.Fatalf("unexpected explicit date args: %#v", args)
	}
}

func TestPasswordHashAndChangeValidation(t *testing.T) {
	hash, err := hashPassword("old-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(hash, "old-secret") {
		t.Fatal("password hash should verify the original password")
	}
	if verifyPassword(hash, "wrong-secret") {
		t.Fatal("password hash must reject wrong password")
	}

	nextHash, err := changePasswordHash(hash, "old-secret", "new-secret")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(nextHash, "new-secret") {
		t.Fatal("changed password hash should verify the new password")
	}
	if _, err := changePasswordHash(hash, "wrong-secret", "new-secret"); err == nil {
		t.Fatal("changing password should require the current password")
	}
}
