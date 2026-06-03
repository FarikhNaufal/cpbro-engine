package usecase

import "testing"

func TestUnionActiveSymbols_PreservesActivePriority(t *testing.T) {
	got := unionActiveSymbols(
		[]string{"ethusdt", "BTCUSDT"},
		[]string{"ADAUSDT", "ETHUSDT", "SOLUSDT"},
	)

	want := []string{"ETHUSDT", "BTCUSDT", "ADAUSDT", "SOLUSDT"}
	if len(got) != len(want) {
		t.Fatalf("expected %d symbols, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected symbol order at %d: got=%s want=%s full=%#v", i, got[i], want[i], got)
		}
	}
}

func TestCollectActiveJournalSymbols_OnlyTracksActiveStatuses(t *testing.T) {
	got := collectActiveJournalSymbols(
		[]SignalJournal{
			{Symbol: "btcusdt", Status: MONITORING},
			{Symbol: "ethusdt", Status: EXPIRED},
			{Symbol: "solusdt", Status: TP1_HIT},
		},
		[]WatchJournal{
			{Symbol: "adausdt", Status: WATCH_MONITORING},
			{Symbol: "xrpusdt", Status: VIRTUAL_EXPIRED},
			{Symbol: "dogeusdt", Status: VIRTUAL_TP1_HIT},
		},
	)

	want := []string{"BTCUSDT", "SOLUSDT", "ADAUSDT", "DOGEUSDT"}
	if len(got) != len(want) {
		t.Fatalf("expected %d symbols, got %d: %#v", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected symbol order at %d: got=%s want=%s full=%#v", i, got[i], want[i], got)
		}
	}
}
