package anima

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"testing"

	"github.com/fxcl/anima/dialect"
)

// Coverage backfill: paths the implementation already supports but that review found had zero
// cases (config surface, in-memory truncation, the And/Or raw condition chains, ResultKey
// conversions, and Save's skipping of nil/zero-value columns).

// captureSQL temporarily enables the SQL statistic log and returns a function for retrieving the log text.
func captureSQL(t *testing.T) func() string {
	t.Helper()
	var buf strings.Builder
	oldOut := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	Of().EnableSQLStatistic(true)
	return func() string {
		log.SetOutput(oldOut)
		log.SetFlags(oldFlags)
		Of().EnableSQLStatistic(false)
		return buf.String()
	}
}

func TestUseSQLLimitBothPaths(t *testing.T) {
	ctx := context.Background()
	seedUsers(t) // ann(30) bob(20) cat(10)

	// Path one: limit is spliced into the SQL (default useSQLLimit=true)
	done := captureSQL(t)
	list, err := Select[User]().Limit(2).Order("id asc").All(ctx)
	sqlText := done()
	if err != nil || len(list) != 2 || list[0].UserName != "ann" {
		t.Fatalf("sql limit: %v %+v", err, list)
	}
	if !strings.Contains(sqlText, "LIMIT ?") {
		t.Fatalf("expect SQL-level limit, logged: %s", sqlText)
	}

	// Path two: in-memory truncation (useSQLLimit=false)
	a := Of()
	a.UseSQLLimit(false)
	defer a.UseSQLLimit(true)
	done = captureSQL(t)
	list, err = Select[User]().Limit(2).Order("id asc").All(ctx)
	sqlText = done()
	if err != nil || len(list) != 2 || list[0].UserName != "ann" {
		t.Fatalf("memory limit: %v %+v", err, list)
	}
	if strings.Contains(sqlText, "LIMIT") {
		t.Fatalf("memory truncation must not emit SQL limit: %s", sqlText)
	}
}

func TestOpenDBWiresInstance(t *testing.T) {
	orig := Of()
	defer SetDefault(orig)

	dsn := fmt.Sprintf("root@tcp(127.0.0.1:%d)/%s?parseTime=true", testPort, testDB)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	a := OpenDB(db)
	if Of() != a || a.db != db {
		t.Fatal("OpenDB must register the given *sql.DB as default")
	}
	seedUsers(t)
	n, err := Select[User]().Count(context.Background())
	if err != nil || n != 3 {
		t.Fatalf("count via OpenDB instance: %v %d", err, n)
	}
}

func TestOpenWithUnknownDriver(t *testing.T) {
	if _, err := OpenWith("no-such-driver", "whatever"); err == nil {
		t.Fatal("unknown driver must error at OpenWith")
	}
}

func TestDialectPaginateEndToEnd(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	a := Of()
	a.Dialect(dialect.PostgreSQL{})
	defer a.Dialect(dialect.MySQL{})

	done := captureSQL(t)
	page, err := Select[User]().Order("id asc").Page(ctx, 1, 2)
	sqlText := done()
	if err != nil {
		// go-mysql-server is a MySQL engine and does not accept PG-style double-quoted identifiers;
		// on real PG there would be no error. Only verify row data when the engine can run the query,
		// otherwise skip.
		if strings.Contains(sqlText, "\"users\"") {
			t.Skipf("engine does not accept PG-style identifier quoting (expected on real PG); SQL was %s", sqlText)
		}
		t.Fatal(err)
	}
	if len(page.Rows) != 2 || page.Rows[0].UserName != "ann" {
		t.Fatalf("pg page rows: %+v", page.Rows)
	}
	if !strings.Contains(sqlText, "LIMIT 2 OFFSET 0") {
		t.Fatalf("expect PostgreSQL paginate SQL, logged: %s", sqlText)
	}
}

func TestAndOrRawConditions(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	n, err := Select[User]().Where("age >= ?", 20).And("age <= ?", 20).Count(ctx)
	sqlText := done()
	if err != nil || n != 1 {
		t.Fatalf("and: %v %d", err, n)
	}
	if !strings.Contains(sqlText, "age >= ? AND age <= ?") {
		t.Fatalf("and join shape: %s", sqlText)
	}

	done = captureSQL(t)
	n, err = Select[User]().Where("age > ?", 25).Or("age < ?", 15).Count(ctx)
	sqlText = done()
	if err != nil || n != 2 { // ann(30) and cat(10)
		t.Fatalf("or: %v %d", err, n)
	}
	if !strings.Contains(sqlText, "age > ? OR (age < ?)") {
		t.Fatalf("or join shape: %s", sqlText)
	}
}

func TestResultKeyConversions(t *testing.T) {
	ctx := context.Background()
	if _, err := Execute(ctx, "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	k, err := Save(ctx, &User{UserName: "keytest", Age: 1})
	if err != nil {
		t.Fatal(err)
	}
	if k.AsInt() != int(k.AsLong()) || k.AsLong() <= 0 {
		t.Fatalf("key = %+v", k)
	}
	if got, want := k.String(), strconv.FormatInt(k.AsLong(), 10); got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestSaveSkipsNilAndZeroPKColumns(t *testing.T) {
	ctx := context.Background()
	if _, err := Execute(ctx, "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}

	done := captureSQL(t)
	k, err := Save(ctx, &User{UserName: "minimal", Age: 7})
	sqlText := done()
	if err != nil || k.AsLong() <= 0 {
		t.Fatalf("save: %v %+v", err, k)
	}
	if !strings.Contains(sqlText, "INSERT INTO `users` (`user_name`, `age`)") {
		t.Fatalf("nil/zero columns must be skipped, logged: %s", sqlText)
	}
	if strings.Contains(sqlText, "email") || strings.Contains(sqlText, "created_at") {
		t.Fatalf("nil columns leaked into INSERT: %s", sqlText)
	}

	one, err := Select[User]().ById(ctx, k.AsLong())
	if err != nil || one == nil || one.UserName != "minimal" || one.Email != nil {
		t.Fatalf("roundtrip: %v %+v", err, one)
	}
}

func TestOrderColWithoutDir(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	one, err := Select[User]().OrderCol("Id", "").One(ctx)
	sqlText := done()
	if err != nil || one == nil {
		t.Fatalf("order: %v %+v", err, one)
	}
	if !strings.Contains(sqlText, "ORDER BY id") {
		t.Fatalf("expect plain column order: %s", sqlText)
	}
	if strings.Contains(sqlText, "id asc") || strings.Contains(sqlText, "id desc") {
		t.Fatalf("empty dir must not append direction: %s", sqlText)
	}
}
