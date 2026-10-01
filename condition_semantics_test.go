package anima

import (
	"strings"
	"testing"
)

// Condition-semantics pins: the exact SQL shapes and edge behaviors of the
// WhereCol/Or condition builder that nothing else asserts.
func TestConditionSemanticsPins(t *testing.T) {
	ctx := t.Context()
	ids := seedUsers(t)

	// Between joins its bounds with a lowercase "and" (Java-parity shape),
	// and a reversed range is valid SQL that matches nothing — (0 rows,
	// nil error), not a validation failure.
	done := captureSQL(t)
	list, err := Select[User]().WhereCol("Age").Between(30, 10).All(ctx)
	sqlText := done()
	if err != nil || len(list) != 0 {
		t.Fatalf("reversed between must match nothing, got (%d rows, %v)", len(list), err)
	}
	if !strings.Contains(sqlText, "BETWEEN ? and ?") {
		t.Fatalf("between shape: %s", sqlText)
	}

	// A nil element inside In renders its own placeholder; plain SQL NULL
	// comparison semantics apply (NULL never matches), so only the non-nil
	// id comes back.
	list, err = Select[User]().WhereCol("Id").In(ids[0], nil).All(ctx)
	if err != nil || len(list) != 1 || list[0].Id != ids[0] {
		t.Fatalf("in with nil element: %v %+v", err, list)
	}

	// Or wraps its raw fragment in parens, so an AND inside the fragment
	// stays bound to the OR branch: "age > ? OR (age < ? AND user_name <> ?)".
	// Seed ages are 30/10/20: only ann(30) survives the <> 'cat' filter.
	done = captureSQL(t)
	n, err := Select[User]().WhereCol("Age").Gt(25).Or("age < ? AND user_name <> ?", 15, "cat").Count(ctx)
	sqlText = done()
	if err != nil || n != 1 {
		t.Fatalf("or-with-and: %v %d", err, n)
	}
	if !strings.Contains(sqlText, "age > ? OR (age < ? AND user_name <> ?)") {
		t.Fatalf("or-with-and shape: %s", sqlText)
	}

	// Or as the FIRST condition still parenthesizes: WHERE (age < ?).
	done = captureSQL(t)
	n, err = Select[User]().Or("age < ?", 15).Count(ctx)
	sqlText = done()
	if err != nil || n != 1 {
		t.Fatalf("or-first: %v %d", err, n)
	}
	if !strings.Contains(sqlText, "WHERE (age < ?)") {
		t.Fatalf("or-first shape: %s", sqlText)
	}
}
