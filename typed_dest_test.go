package anima

import (
	"testing"
	"time"
)

// TestTypedDestPointerNullAndValue pins the NULL semantics of the typed scan
// destinations in scanReflect. Pointer fields (Email *string, CreatedAt
// *time.Time) are scanned into typed destinations, so database/sql performs the
// conversion instead of convertAssign. A NULL column must still map to a nil
// pointer and a present value must still map to a populated one.
func TestTypedDestPointerNullAndValue(t *testing.T) {
	ctx := t.Context()

	const nullName = "typed_dest_null"
	const valueName = "typed_dest_value"

	if _, err := Execute(ctx, "DELETE FROM users WHERE user_name IN (?, ?)", nullName, valueName); err != nil {
		t.Fatalf("cleanup: %v", err)
	}

	// NULL email and NULL created_at.
	if _, err := Execute(ctx,
		"INSERT INTO users (user_name, age, email, created_at) VALUES (?, ?, ?, ?)",
		nullName, 1, nil, nil); err != nil {
		t.Fatalf("insert null row: %v", err)
	}

	when := time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)
	mail := "typed@example.com"
	if _, err := Execute(ctx,
		"INSERT INTO users (user_name, age, email, created_at) VALUES (?, ?, ?, ?)",
		valueName, 2, mail, when); err != nil {
		t.Fatalf("insert value row: %v", err)
	}

	rows, err := Select[User]().Where("user_name = ?", nullName).All(ctx)
	if err != nil {
		t.Fatalf("select null row: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("null row: got %d rows, want 1", len(rows))
	}
	if rows[0].Email != nil {
		t.Errorf("NULL email mapped to %q, want nil pointer", *rows[0].Email)
	}
	if rows[0].CreatedAt != nil {
		t.Errorf("NULL created_at mapped to %v, want nil pointer", *rows[0].CreatedAt)
	}

	rows, err = Select[User]().Where("user_name = ?", valueName).All(ctx)
	if err != nil {
		t.Fatalf("select value row: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("value row: got %d rows, want 1", len(rows))
	}
	if rows[0].Email == nil {
		t.Fatal("present email mapped to nil pointer, want a value")
	}
	if *rows[0].Email != mail {
		t.Errorf("email = %q, want %q", *rows[0].Email, mail)
	}
	if rows[0].CreatedAt == nil {
		t.Fatal("present created_at mapped to nil pointer, want a value")
	}
	if !rows[0].CreatedAt.Equal(when) {
		t.Errorf("created_at = %v, want %v", *rows[0].CreatedAt, when)
	}

	if _, err := Execute(ctx, "DELETE FROM users WHERE user_name IN (?, ?)", nullName, valueName); err != nil {
		t.Fatalf("cleanup: %v", err)
	}
}
