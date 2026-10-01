package anima

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// This file guards the fixes for the review-and-simplify-changes group B behavior gaps.
// It first landed in "expected-failure" form (TDD) and is now fully green together with the fixes:
//   - BySQL fails fast in update/delete mode
//   - Read terminals (All/One/Count/ById) fail fast in update/delete mode
//   - Limit(0) selects nothing; Limit(-1) errors (alignment guard for Java limit(n) semantics)
//   - Page(pageNum<1 / pageSize<1) input validation
//   - Execute() checks the stored error at entry, so mode errors no longer mask build-time errors
//   - convertAssign narrows numerics to the target width and errors on overflow (aligned with database/sql)
// If any later change breaks one of these, this file acts as the regression alarm.

func TestGapBySQLRejectedInUpdateMode(t *testing.T) {
	ctx := context.Background()
	if _, err := Execute(ctx, "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	_, err := UpdateFrom[User]().Set("Age", 9).BySQL("UPDATE users SET age = 8").Execute(ctx)
	if err == nil {
		t.Fatal("BySQL must fail fast in update/delete mode")
	}
}

func TestGapReadTerminalRejectedInUpdateMode(t *testing.T) {
	ctx := context.Background()
	if _, err := Execute(ctx, "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateFrom[User]().Set("Age", 1).All(ctx); err == nil {
		t.Fatal("read terminal on update-mode query must fail fast")
	}
	if _, err := DeleteFrom[User]().One(ctx); err == nil {
		t.Fatal("read terminal on delete-mode query must fail fast")
	}
}

func TestGapLimitNonPositive(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	list, err := Select[User]().Limit(0).All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("Limit(0) must select nothing, got %d rows", len(list))
	}
	if _, err := Select[User]().Limit(-1).All(ctx); err == nil {
		t.Fatal("Limit(-1) must error")
	}
}

func TestGapPageInputValidation(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)
	if _, err := Select[User]().Page(ctx, 0, 10); err == nil {
		t.Fatal("pageNum < 1 must error")
	}
	if _, err := Select[User]().Page(ctx, 1, 0); err == nil {
		t.Fatal("pageSize < 1 must error")
	}
}

func TestGapExecuteReturnsStoredError(t *testing.T) {
	_, err := Select[User]().WhereCol("Nope").Eq(1).Execute(context.Background())
	var e *Error
	if !errors.As(err, &e) || e.Code != ErrorCodeUnknownField {
		t.Fatalf("want UnknownField stored error, got %v", err)
	}
}

// Out-of-range values into small integer kinds: negative int64 into uint wraps (-1->255),
// large int64 into int8 wraps (200->-56), float truncates (15.9->15), and strings parsed
// as 64-bit into sized fields wrap ("300"->uint8 gives 44). The expected semantics align with
// database/sql convertAssign: parse at the target width and error on overflow.
//
// Why white-box direct tests: go-mysql-server speaks the text protocol, so negative values
// always arrive as strings. The end-to-end channel can only exercise the string branch and
// never reaches the int64/float64 direct branches.
func TestGapNumericRangeIntoSmallIntKinds(t *testing.T) {
	cases := []struct {
		name string
		dst  any
		src  any
	}{
		{"int64(-1)->uint8", new(uint8), int64(-1)},
		{"int64(200)->int8", new(int8), int64(200)},
		{"float64(15.9)->int", new(int), float64(15.9)},
		{`string("300")->uint8`, new(uint8), "300"},
		{`string("200")->int8`, new(int8), "200"},
	}
	for _, c := range cases {
		dst := reflect.ValueOf(c.dst).Elem()
		if err := convertAssign(dst, c.src, nil); err == nil {
			t.Errorf("%s: want range error, got %v", c.name, dst.Interface())
		}
	}
	// Control: in-range values must still convert normally
	if err := convertAssign(reflect.ValueOf(new(uint8)).Elem(), int64(255), nil); err != nil {
		t.Errorf("in-range int64->uint8: %v", err)
	}
}
