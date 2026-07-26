package main

import "testing"

func resetMaps() {
	fakeToOriginalChecksum = make(map[string]string)
	originalToFakeChecksum = make(map[string]string)
}

// Two gallery copies of the same picture convert to a byte-identical file. Both must keep
// resolving to that file, otherwise bulk-upload-check misses and the app re-uploads forever.
func TestDistinctOriginalsSharingOneFake(t *testing.T) {
	resetMaps()
	if !setChecksumPair("F", "A") {
		t.Fatal("first pair should be recorded")
	}
	if !setChecksumPair("F", "B") {
		t.Fatal("second original for the same fake should be recorded")
	}
	for _, original := range []string{"A", "B"} {
		if got := originalToFakeChecksum[original]; got != "F" {
			t.Errorf("original %q resolves to %q, want \"F\"", original, got)
		}
	}
	// One server asset can only report one checksum; it must not flip between copies.
	if got := fakeToOriginalChecksum["F"]; got != "A" {
		t.Errorf("reverse lookup is %q, want the first original \"A\"", got)
	}
}

// The ping-pong that stalled the queue: A, then B, then A again.
func TestAlternatingUploadsKeepBothMapped(t *testing.T) {
	resetMaps()
	setChecksumPair("F", "A")
	setChecksumPair("F", "B")
	setChecksumPair("F", "A")
	if len(originalToFakeChecksum) != 2 {
		t.Fatalf("originalToFakeChecksum has %d entries, want 2", len(originalToFakeChecksum))
	}
	if fakeToOriginalChecksum["F"] != "A" {
		t.Errorf("reverse lookup flipped to %q", fakeToOriginalChecksum["F"])
	}
}

// A repeat of a pair already on file must not be appended to the CSV again.
func TestKnownPairIsNotRecordedTwice(t *testing.T) {
	resetMaps()
	setChecksumPair("F", "A")
	if setChecksumPair("F", "A") {
		t.Error("re-recording a known pair should report no change")
	}
}

// The reverse direction is genuinely 1:1: if one original starts converting to a different
// file, the stale reverse entry has to go or the old asset keeps claiming that original.
func TestOriginalRemappedToNewFake(t *testing.T) {
	resetMaps()
	setChecksumPair("F1", "A")
	if !setChecksumPair("F2", "A") {
		t.Fatal("remapping an original to a new fake should be recorded")
	}
	if got := originalToFakeChecksum["A"]; got != "F2" {
		t.Errorf("original resolves to %q, want \"F2\"", got)
	}
	if _, ok := fakeToOriginalChecksum["F1"]; ok {
		t.Error("stale reverse entry for F1 was not removed")
	}
}

// Restarting replays the CSV through the same function, so the rebuilt maps must match.
func TestCSVReplayRebuildsBothOriginals(t *testing.T) {
	resetMaps()
	for _, pair := range [][2]string{{"F", "A"}, {"F", "B"}, {"G", "C"}} {
		setChecksumPair(pair[0], pair[1])
	}
	for original, want := range map[string]string{"A": "F", "B": "F", "C": "G"} {
		if got := originalToFakeChecksum[original]; got != want {
			t.Errorf("after replay, original %q resolves to %q, want %q", original, got, want)
		}
	}
}
