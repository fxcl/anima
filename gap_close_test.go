package anima

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/fxcl/anima/dialect"
)

// Tests closing the inventoried Java-API gaps (autoresearch/manifest.tsv).
// Each test mirrors the corresponding Java behavior in com.hellokaton.anima.

func TestGapCloseResultKeyAsBigInteger(t *testing.T) {
	rk := &ResultKey{key: 9007199254740993}
	got := rk.AsBigInteger()
	if got == nil {
		t.Fatal("AsBigInteger must not return nil")
	}
	if got.Cmp(big.NewInt(9007199254740993)) != 0 {
		t.Fatalf("AsBigInteger = %s, want 9007199254740993", got)
	}
	if again := rk.AsBigInteger(); again.Cmp(got) != 0 {
		t.Fatalf("AsBigInteger unstable: %s vs %s", again, got)
	}
}

func TestGapCloseCondNotEmpty(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	list, err := Select[User]().WhereCol("UserName").NotEmpty().All(ctx)
	sqlText := done()
	if err != nil {
		t.Fatalf("NotEmpty query: %v", err)
	}
	if !strings.Contains(sqlText, "user_name != ''") {
		t.Fatalf("NotEmpty must emit Java shape on the mapped column: %s", sqlText)
	}
	if len(list) == 0 {
		t.Fatal("expected non-empty names to match")
	}
}

func TestGapCloseAnimaGetters(t *testing.T) {
	a := Of()
	origPrefix := a.GetTablePrefix()
	origDialect := a.GetDialect()
	defer func() { a.TablePrefix(origPrefix).Dialect(origDialect) }()

	a.TablePrefix("t_").Dialect(dialect.PostgreSQL{})
	if a.GetTablePrefix() != "t_" {
		t.Fatalf("GetTablePrefix = %q, want t_", a.GetTablePrefix())
	}
	if _, ok := a.GetDialect().(dialect.PostgreSQL); !ok {
		t.Fatalf("GetDialect = %T, want dialect.PostgreSQLDialect", a.GetDialect())
	}
	if !a.IsUseSQLLimit() {
		t.Fatal("default useSQLLimit must be true")
	}
	// TestMain silences SQL statistics for the shared default instance; assert
	// the getter reads back the setter rather than pinning a process-wide value.
	prev := a.IsEnableSQLStatistic()
	a.EnableSQLStatistic(!prev)
	if a.IsEnableSQLStatistic() == prev {
		t.Fatal("IsEnableSQLStatistic must read back EnableSQLStatistic")
	}
	a.EnableSQLStatistic(prev)
}

func TestGapCloseQueryMapFilter(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	names, err := Map(ctx, Select[User]().WhereCol("Age").Gt(1), func(u *User) string { return u.UserName })
	if err != nil {
		t.Fatalf("Map: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("Map rows = %d, want 3", len(names))
	}

	kept, err := Select[User]().WhereCol("Age").Gt(1).Filter(ctx, func(u *User) bool { return u.Age > 15 })
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if len(kept) != 2 {
		t.Fatalf("Filter must keep the two rows with Age > 15, got %d", len(kept))
	}
}

func TestGapCloseQuerySeq(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	seq, err := Select[User]().WhereCol("Age").Gt(1).Seq(ctx)
	if err != nil {
		t.Fatalf("Seq: %v", err)
	}
	count := 0
	var first *User
	for u := range seq {
		if count == 0 {
			first = u
		}
		count++
	}
	if count != 3 {
		t.Fatalf("Seq rows = %d, want 3", count)
	}
	if first == nil || first.UserName == "" {
		t.Fatalf("Seq must yield mapped entities, got %+v", first)
	}

	empty, err := Select[User]().WhereCol("Age").Gt(999).Seq(ctx)
	if err != nil {
		t.Fatalf("Seq empty: %v", err)
	}
	for range empty {
		t.Fatal("empty Seq must yield nothing")
	}
}

func TestGapCloseEnglishModes(t *testing.T) {
	if got := English(EnglishAnglicized).Plural("octopus"); got != "octopuses" {
		t.Fatalf("anglicized octopus = %q, want octopuses", got)
	}
	if got := English(EnglishClassical).Plural("octopus"); got != "octopodes" {
		t.Fatalf("classical octopus = %q, want octopodes", got)
	}
	if got := English(EnglishClassical).Plural("money"); got != "monies" {
		t.Fatalf("classical money = %q, want monies", got)
	}
}

func TestGapCloseEnglishPluralCount(t *testing.T) {
	if got := English(EnglishAnglicized).PluralCount("cat", 1); got != "cat" {
		t.Fatalf("PluralCount(cat, 1) = %q, want cat", got)
	}
	if got := English(EnglishAnglicized).PluralCount("cat", 2); got != "cats" {
		t.Fatalf("PluralCount(cat, 2) = %q, want cats", got)
	}
	if got := English(EnglishAnglicized).PluralCount("cat", 0); got != "cats" {
		t.Fatalf("PluralCount(cat, 0) = %q, want cats (Java pluralizes for any count != 1)", got)
	}
	if got := English(EnglishClassical).PluralCount("octopus", 2); got != "octopodes" {
		t.Fatalf("PluralCount(octopus, 2) classical = %q, want octopodes", got)
	}
}

func TestGapCloseNamingUtils(t *testing.T) {
	if got := ToUnderline("UserName"); got != "user_name" {
		t.Fatalf("ToUnderline(UserName) = %q, want user_name", got)
	}
	if got := ToUnderline("userId"); got != "user_id" {
		t.Fatalf("ToUnderline(userId) = %q, want user_id", got)
	}
	if got := ToCamelName("user_name"); got != "userName" {
		t.Fatalf("ToCamelName(user_name) = %q, want userName", got)
	}
	if got := ToCamelName("user_id"); got != "userId" {
		t.Fatalf("ToCamelName(user_id) = %q, want userId", got)
	}
}
