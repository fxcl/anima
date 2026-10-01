package anima

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// Java behavior alignment guards: behavior differences found and fixed against com.hellokaton.anima,
// locked down here to prevent regression:
//   - Or() group parens (Java " OR (stmt)"; precedence semantics when mixed with AND)
//   - In() with no args skips the whole condition (Java log.warn + return this; never emits "IN ()")
//   - Set() with the same column overwrites (Java updateColumns is a LinkedHashMap)
//   - Page count SQL carries no ORDER BY (Java buildSelectSQL(false); MySQL tolerates it, PG errors)
//   - ById queries carry no ORDER BY (Java byId goes through buildSelectSQL(false))
//   - Atomic default RollbackWhen triggers only on RuntimeException (alignment guard for Java rollbackException = RuntimeException.class)
//   - Order/OrderCol comma-appends on repeated calls (Java orderBySQL append rule)
//   - Atomic returning an error when the transaction fails to begin (e.g. OpenDB(nil)) yields AtomicResult{err}, not a panic

func TestAlignOrWrapsGroup(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	n, err := Select[User]().Where("age > ?", 25).Or("age < ?", 15).Count(ctx)
	sqlText := done()
	if err != nil || n != 2 {
		t.Fatalf("or query: %v %d", err, n)
	}
	if !strings.Contains(sqlText, "age > ? OR (age < ?)") {
		t.Fatalf("Or must wrap in parens (Java \" OR (stmt)\"): %s", sqlText)
	}
}

func TestAlignInEmptySkips(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	list, err := Select[User]().WhereCol("Age").In().All(ctx)
	sqlText := done()
	if err != nil {
		t.Fatalf("empty In must skip the condition, not emit \"IN ()\": %v", err)
	}
	if strings.Contains(sqlText, "IN ()") {
		t.Fatalf("empty In must not emit IN (): %s", sqlText)
	}
	if len(list) != 3 { // ann(30) bob(20) cat(10), all rows returned
		t.Fatalf("want all rows when condition skipped, got %d", len(list))
	}
}

func TestAlignSetOverwritesSameColumn(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	if _, err := UpdateFrom[User]().WhereCol("Age").Eq(30).Set("Age", 1).Set("Age", 2).Set("UserName", "x").Execute(ctx); err != nil {
		t.Fatal(err)
	}
	sqlText := done()
	if !strings.Contains(sqlText, "SET `age` = ?, `user_name` = ?") {
		t.Fatalf("Set must overwrite same column (Java LinkedHashMap): %s", sqlText)
	}
	u, err := Select[User]().WhereCol("UserName").Eq("x").One(ctx)
	if err != nil || u == nil || u.Age != 2 {
		t.Fatalf("last Set must win: %+v %v", u, err)
	}
}

func TestAlignPageCountStripsOrderBy(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	if _, err := Select[User]().Order("age desc").Page(ctx, 1, 10); err != nil {
		t.Fatal(err)
	}
	sqlText := done()
	for _, line := range strings.Split(sqlText, "\n") {
		if strings.Contains(line, "COUNT(*)") && strings.Contains(line, "ORDER BY") {
			t.Fatalf("page count SQL must not carry ORDER BY: %s", line)
		}
	}
	if !strings.Contains(sqlText, "ORDER BY age desc") {
		t.Fatalf("data page SQL must keep ORDER BY: %s", sqlText)
	}
}

func TestAlignByIdIgnoresOrder(t *testing.T) {
	ctx := context.Background()
	if _, err := Execute(ctx, "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	k, err := Save(ctx, &User{UserName: "byid", Age: 42})
	if err != nil {
		t.Fatal(err)
	}

	done := captureSQL(t)
	u, err := Select[User]().Order("age desc").ById(ctx, k.AsLong())
	sqlText := done()
	if err != nil || u == nil {
		t.Fatalf("byId: %v", err)
	}
	if strings.Contains(sqlText, "ORDER BY") {
		t.Fatalf("byId must not emit ORDER BY (Java buildSelectSQL(false)): %s", sqlText)
	}
}

// Feature-parity guards (Java had these; Go previously lacked them):
//   - Between (Java between(col, a, b))
//   - Exclude for excluded columns (Java exclude(...); excluding all falls back to "*"; Cols wins)
//   - Maps (Java maps())
//   - ByIds (Java byIds(...); an empty call mirrors the whole-table quirk of Java in() skipping the condition and then all())
//   - WhereModel (Java where(T model): nil/empty-string skips)
//   - Cols called twice errors (Java throws "Select method can only be called once.")
//   - Page.RecalcNavPages / Peek (Java navPages(int) / peek(consumer))

func TestAlignBetween(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	list, err := Select[User]().WhereCol("Age").Between(15, 25).All(ctx)
	sqlText := done()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, "BETWEEN ? and ?") {
		t.Fatalf("Between must emit Java's lowercase shape: %s", sqlText)
	}
	if len(list) != 1 || list[0].UserName != "bob" { // ann(30) bob(20) cat(10)
		t.Fatalf("want [bob], got %+v", list)
	}
}

func TestAlignExclude(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	list, err := Select[User]().Exclude("Email").All(ctx)
	sqlText := done()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, "SELECT id, user_name, age, created_at FROM `users`") {
		t.Fatalf("exclude column shape: %s", sqlText)
	}
	if len(list) != 3 || list[0].Email != nil {
		t.Fatalf("excluded column must stay nil: %d rows", len(list))
	}

	// Excluding all columns falls back to "*" (alignment guard for Java buildColumns producing an empty result)
	done = captureSQL(t)
	if _, err := Select[User]().Exclude("Id", "UserName", "Age", "Email", "CreatedAt").All(ctx); err != nil {
		t.Fatal(err)
	}
	if sqlText = done(); !strings.Contains(sqlText, "SELECT * FROM `users`") {
		t.Fatalf("exclude-all must fall back to *: %s", sqlText)
	}

	// Cols takes precedence over Exclude (alignment guard for Java checking the selectColumns branch first)
	done = captureSQL(t)
	if _, err := Select[User]().Cols("Age").Exclude("Email").All(ctx); err != nil {
		t.Fatal(err)
	}
	if sqlText = done(); !strings.Contains(sqlText, "SELECT age FROM `users`") {
		t.Fatalf("Cols must win over Exclude: %s", sqlText)
	}
}

func TestAlignMaps(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	list, err := Select[User]().WhereCol("UserName").Eq("ann").Maps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 row, got %d", len(list))
	}
	for _, key := range []string{"id", "user_name", "age", "email", "created_at"} {
		if _, ok := list[0][key]; !ok {
			t.Fatalf("maps row missing key %q: %v", key, list[0])
		}
	}
}

func TestAlignByIds(t *testing.T) {
	ctx := context.Background()
	ids := seedUsers(t)

	list, err := Select[User]().ByIds(ctx, ids[0], ids[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("want 2 rows by ids, got %d", len(list))
	}
	// Java quirk: byIds() with no args -> in() skips the condition -> all() returns the whole table
	list, err = Select[User]().ByIds(ctx)
	if err != nil || len(list) != 3 {
		t.Fatalf("empty ByIds must return whole table (Java quirk): %d %v", len(list), err)
	}
}

func TestAlignWhereModel(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	list, err := Select[User]().WhereModel(&User{UserName: "ann", Age: 0}).All(ctx)
	sqlText := done()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sqlText, "WHERE id = ? AND user_name = ? AND age = ?") {
		t.Fatalf("where(model) must keep zero non-nil fields (Java getter != null): %s", sqlText)
	}
	if len(list) != 0 {
		t.Fatalf("ann has age 30, want 0 rows, got %d", len(list))
	}

	// Empty-string fields are skipped (same as Java isEmpty(value.toString()))
	done = captureSQL(t)
	if _, err := Select[User]().WhereModel(&User{UserName: "", Age: 30}).All(ctx); err != nil {
		t.Fatal(err)
	}
	if sqlText = done(); strings.Contains(sqlText, "user_name") {
		t.Fatalf("empty string field must be skipped: %s", sqlText)
	}
}

func TestAlignColsTwice(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)
	if _, err := Select[User]().Cols("Age").Cols("UserName").All(ctx); err == nil {
		t.Fatal("Cols twice must error (Java: Select method can only be called once.)")
	}
}

func TestAlignPageNavAndPeek(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	p, err := Select[User]().Order("age desc").Page(ctx, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(p.Rows))
	}
	seen := 0
	if p.Peek(func(u *User) { seen++ }) != p {
		t.Fatal("Peek must return same page (Java peek returns this)")
	}
	if seen != 2 {
		t.Fatalf("Peek visited %d rows", seen)
	}
	p2 := NewPage[User](105, 1, 10).RecalcNavPages(4)
	if len(p2.NavPageNums) != 4 {
		t.Fatalf("RecalcNavPages(4) want 4 nav nums, got %v", p2.NavPageNums)
	}
}

func TestAlignOrderAccumulates(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	if _, err := Select[User]().Order("age").Order("user_name desc").All(ctx); err != nil {
		t.Fatal(err)
	}
	if sqlText := done(); !strings.Contains(sqlText, "ORDER BY age, user_name desc") {
		t.Fatalf(`Order("age").Order("user_name desc") must accumulate (Java orderBySQL): %s`, sqlText)
	}

	done = captureSQL(t)
	if _, err := Select[User]().Order("age").OrderCol("UserName", "desc").All(ctx); err != nil {
		t.Fatal(err)
	}
	if sqlText := done(); !strings.Contains(sqlText, "ORDER BY age, user_name desc") {
		t.Fatalf("Order + OrderCol must accumulate: %s", sqlText)
	}
}

func TestAtomicNilDBReturnsError(t *testing.T) {
	prev := Of()
	defer SetDefault(prev)
	OpenDB(nil)

	called := false
	res := Atomic(context.Background(), func(ctx context.Context) error {
		called = true
		return nil
	})
	if called {
		t.Fatal("fn must not run when begin fails")
	}
	if res.Error() == nil {
		t.Fatal("nil db must yield AtomicResult error, not panic")
	}
	if res.IsRollback() {
		t.Fatal("no tx was opened, nothing to roll back")
	}
}

func TestAlignResultKeyAsString(t *testing.T) {
	ctx := context.Background()
	if _, err := Execute(ctx, "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	k, err := Save(ctx, &User{UserName: "astring", Age: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := strconv.FormatInt(k.AsLong(), 10)
	if got := k.AsString(); got != want {
		t.Fatalf("AsString = %q, want %q", got, want)
	}
}

func TestAlignAtomicRollbackDefault(t *testing.T) {
	ctx := context.Background()

	// Default: a plain error does not roll back (alignment guard for Java checked exceptions: commit and store the error in the Atomic)
	r := Atomic(ctx, func(ctx context.Context) error { return errors.New("checked-style") })
	if r.Error() == nil || r.IsRollback() {
		t.Fatalf("default RollbackWhen must NOT trigger on plain error: %+v", r)
	}

	// Default: Unchecked(err) rolls back (alignment guard for the Java RuntimeException equivalent)
	r = Atomic(ctx, func(ctx context.Context) error { return Unchecked(errors.New("runtime-style")) })
	if r.Error() == nil || !r.IsRollback() {
		t.Fatalf("Unchecked(err) must trigger rollback: %+v", r)
	}

	// Default: panic rolls back (a panic is wrapped in uncheckedErr, which automatically satisfies RuntimeException)
	r = Atomic(ctx, func(ctx context.Context) error { panic("kaboom") })
	if r.Error() == nil || !r.IsRollback() {
		t.Fatalf("panic must trigger rollback under default: %+v", r)
	}
}
