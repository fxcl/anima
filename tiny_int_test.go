package anima

import (
	"testing"
)

// TinyScore exercises the typed-destination scan path for widths narrower than
// int: int8/int16 columns are part of segment 17's typed scan destinations, and
// no existing entity had a field narrower than int to reach that code.
type TinyScore struct {
	Id  int64
	Val int8
	Big int16
}

// TestTypedDestNarrowIntRoundTrip proves the typed scan destinations read
// TINYINT/SMALLINT columns back exactly, including both int8 boundaries, so
// narrowing stays correct through rows.Scan instead of only through the direct
// convertAssign unit test in contract_gap_test.go.
func TestTypedDestNarrowIntRoundTrip(t *testing.T) {
	ctx := t.Context()

	if _, err := Execute(ctx, "DELETE FROM tiny_scores"); err != nil {
		t.Fatalf("clean tiny_scores: %v", err)
	}

	want := []int8{127, -128, 0, 100, -50}
	for _, v := range want {
		ts := &TinyScore{Val: v, Big: int16(v) * 100}
		if _, err := Save(ctx, ts); err != nil {
			t.Fatalf("save Val=%d: %v", v, err)
		}
	}

	got, err := Select[TinyScore]().All(ctx)
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d rows, want %d", len(got), len(want))
	}
	seen := map[int8]bool{}
	for _, ts := range got {
		if ts.Big != int16(ts.Val)*100 {
			t.Errorf("Big=%d does not match Val=%d (want %d)", ts.Big, ts.Val, int16(ts.Val)*100)
		}
		seen[ts.Val] = true
	}
	for _, v := range want {
		if !seen[v] {
			t.Errorf("Val=%d missing from round-trip result", v)
		}
	}
}
