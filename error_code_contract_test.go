package anima

import (
	"context"
	"errors"
	"testing"
)

// Error-code contract pins: every raise site of the two codes nothing else
// asserts by identity (ModeMismatch 1005, InvalidArgument 1006). The
// existing gap tests only assert err != nil; these pin WHICH contract broke.

func TestErrorCodeContract(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	expectCode := func(t *testing.T, err error, want int, site string) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: expected error", site)
		}
		var aErr *Error
		if !errors.As(err, &aErr) {
			t.Fatalf("%s: not an *Error: %v", site, err)
		}
		if aErr.Code != want {
			t.Fatalf("%s: code = %d, want %d (msg: %s)", site, aErr.Code, want, aErr.Msg)
		}
	}

	// ModeMismatch: write terminals on a select-mode query.
	_, err := Select[User]().Execute(ctx)
	expectCode(t, err, ErrorCodeModeMismatch, "select Execute")

	_, err = Select[User]().Set("Age", 1).All(ctx)
	expectCode(t, err, ErrorCodeModeMismatch, "select Set")

	// ModeMismatch: update terminals on a delete-mode query.
	_, err = DeleteFrom[User]().WhereCol("Id").Eq(1).UpdateById(ctx, 1)
	expectCode(t, err, ErrorCodeModeMismatch, "delete UpdateById")

	// ModeMismatch: read terminal and BySQL on an update-mode query.
	_, err = UpdateFrom[User]().Set("Age", 1).All(ctx)
	expectCode(t, err, ErrorCodeModeMismatch, "update All")

	_, err = UpdateFrom[User]().BySQL("UPDATE users SET age = 1").Execute(ctx)
	expectCode(t, err, ErrorCodeModeMismatch, "update BySQL")

	// InvalidArgument: construction-time contract violations.
	_, err = Select[User]().Cols("Age").Cols("UserName").All(ctx)
	expectCode(t, err, ErrorCodeInvalidArgument, "Cols twice")

	_, err = Select[User]().Limit(-1).All(ctx)
	expectCode(t, err, ErrorCodeInvalidArgument, "negative Limit")

	_, err = UpdateFrom[User]().WhereCol("Id").Eq(1).Execute(ctx)
	expectCode(t, err, ErrorCodeInvalidArgument, "update with nothing to set")

	_, err = Select[User]().Page(ctx, 0, 2)
	expectCode(t, err, ErrorCodeInvalidArgument, "Page(0, 2)")
}

// Page clamps an out-of-range page number to the last page instead of
// returning an empty page: pageNum 99 on 3 rows / pageSize 2 lands on the
// final page with its real rows.
func TestPageBeyondLastClamps(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	p, err := Select[User]().Order("id asc").Page(ctx, 99, 2)
	if err != nil {
		t.Fatal(err)
	}
	if p.TotalRows != 3 || p.TotalPages != 2 {
		t.Fatalf("totals: rows=%d pages=%d", p.TotalRows, p.TotalPages)
	}
	if p.PageNum != 2 {
		t.Fatalf("pageNum = %d, want clamped to 2", p.PageNum)
	}
	if len(p.Rows) != 1 || p.Rows[0].UserName != "cat" {
		t.Fatalf("clamped page must hold the last page's rows, got %+v", p.Rows)
	}
}
