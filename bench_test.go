package anima

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
)

// Benchmark harness for comparing the Go port with the Java original.
// Both sides run against the same external engine process (benchserver,
// go-mysql-server on 127.0.0.1:33099), so the numbers reflect client and ORM
// overhead instead of differences between two database engines.
//
// The suite aims at full feature coverage of the anima API. Case names line up
// with Benchmark.java on the Java side; the Java harness documents the few
// cases that have no counterpart there.
//
// Run: go test -run='^$' -bench=. -benchmem ./anima/
// Prerequisite: the benchserver process must be listening on 127.0.0.1:33099.

const benchDSN = "root@tcp(127.0.0.1:33099)/anima_bench?parseTime=true"

// benchDSNFor lets a variant run change the driver configuration without
// editing the baseline. ANIMA_BENCH_DSN is used verbatim when set; the point
// of the knob is to compare wire formats (e.g. interpolateParams=true) on an
// otherwise identical harness.
func benchDSNFor(b *testing.B) string {
	b.Helper()
	if v := os.Getenv("ANIMA_BENCH_DSN"); v != "" {
		return v
	}
	return benchDSN
}

const benchSeedRows = 100

// benchDeletePool is the size of the delete_targets table. Delete cases rotate
// over it so every iteration removes a row that is still present; the rows are
// re-inserted with the timer stopped (benchUndoDeleteTargets), so the pool
// stays full and the affected-row count is deterministic at every iteration.
// The rotation counters persist across the framework's own b.N=1 probe run and
// the scaled run that follows it, which would otherwise hit the same rows
// twice without the undo keeping them present.
const benchDeletePool = 20000

var (
	benchSeqDeleteById  int64
	benchSeqDeleteModel int64
	benchSeqDeleteBatch int64
)

func benchNextDeleteID(seq *int64, step int64) int64 {
	*seq += step
	return (*seq-step)%benchDeletePool + 1
}

// benchInstance points the global default at the external benchserver and
// restores the previous default afterwards.
func benchInstance(b *testing.B) {
	b.Helper()
	prev := Of()
	a, err := Open(benchDSNFor(b))
	if err != nil {
		b.Fatal(err)
	}
	a.EnableSQLStatistic(false) // per-statement logging would dominate the measurement
	b.Cleanup(func() { SetDefault(prev) })
}

// benchReset rewrites every table the assertions depend on. delete_targets is
// deliberately left alone: the delete cases drain that pool by design. Each
// table is restored with a single multi-row INSERT to keep the reset itself
// cheap relative to the measured iterations.
func benchReset(b *testing.B) {
	b.Helper()
	ctx := context.Background()

	var users, prefixUsers, categories, orders strings.Builder
	users.WriteString("INSERT INTO users (id, user_name, age) VALUES ")
	prefixUsers.WriteString("INSERT INTO bench_bench_users (id, user_name, age) VALUES ")
	categories.WriteString("INSERT INTO categories (id, name) VALUES ")
	orders.WriteString("INSERT INTO orders (id, uid, product_name, create_time) VALUES ")
	for i := 1; i <= benchSeedRows; i++ {
		if i > 1 {
			users.WriteString(", ")
		}
		fmt.Fprintf(&users, "(%d, 'user_%03d', %d)", i, i, i)
	}
	for i := 1; i <= 20; i++ {
		if i > 1 {
			prefixUsers.WriteString(", ")
		}
		fmt.Fprintf(&prefixUsers, "(%d, 'bench_%03d', %d)", i, i, 20+i)
	}
	for i := 1; i <= 20; i++ {
		if i > 1 {
			categories.WriteString(", ")
		}
		fmt.Fprintf(&categories, "(%d, 'cat_%03d')", i, i)
	}
	for i := 1; i <= benchSeedRows; i++ {
		if i > 1 {
			orders.WriteString(", ")
		}
		fmt.Fprintf(&orders, "(%d, %d, 'prod_%03d', '2026-01-01 00:00:00')", i, 1+(i%20), i)
	}

	statements := []string{
		"DELETE FROM users",
		"DELETE FROM bench_bench_users",
		"DELETE FROM categories",
		"DELETE FROM orders",
		"DELETE FROM profiles",
		users.String(),
		prefixUsers.String(),
		categories.String(),
		orders.String(),
	}
	for i := 1; i <= benchSeedRows; i++ {
		// Every third profile row leaves the enum and text columns NULL, which
		// the whereIsNull read cases count on.
		nickname, sex, vip := "NULL", "NULL", "NULL"
		if i%3 != 0 {
			nickname = fmt.Sprintf("'nick_%03d'", i)
			if i%2 == 0 {
				sex = "2"
			} else {
				sex = "1"
			}
			vip = fmt.Sprint(1 + (i % 3))
		}
		if i == 1 {
			statements = append(statements,
				"INSERT INTO profiles (id, user_id, nickname, sex, vip) VALUES "+
					fmt.Sprintf("(%d, %d, %s, %s, %s)", i, i, nickname, sex, vip))
		} else {
			statements[len(statements)-1] += ", " +
				fmt.Sprintf("(%d, %d, %s, %s, %s)", i, i, nickname, sex, vip)
		}
	}
	for _, stmt := range statements {
		if _, err := Execute(ctx, stmt); err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// write path
// ---------------------------------------------------------------------------

func BenchmarkSave(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key, err := Save(ctx, &User{UserName: "bench", Age: 30})
		if err != nil {
			b.Fatal(err)
		}
		if key.AsLong() <= 0 {
			b.Fatalf("save: key = %d", key.AsLong())
		}
	}
}

func BenchmarkSavePrefix(b *testing.B) {
	benchInstance(b)
	prev := Of()
	pa, err := Open(benchDSN)
	if err != nil {
		b.Fatal(err)
	}
	pa.EnableSQLStatistic(false).TablePrefix("bench") // BenchUser -> bench_bench_users
	SetDefault(pa)
	b.Cleanup(func() { SetDefault(prev) })
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key, err := Save(ctx, &BenchUser{UserName: "bench", Age: 30})
		if err != nil {
			b.Fatal(err)
		}
		if key.AsLong() <= 0 {
			b.Fatalf("savePrefix: key = %d", key.AsLong())
		}
	}
}

func BenchmarkSaveNull(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key, err := Save(ctx, &Profile{UserId: 2})
		if err != nil {
			b.Fatal(err)
		}
		// The read side must map SQL NULLs back to nil pointers.
		p, err := Select[Profile]().ById(ctx, key.AsLong())
		if err != nil || p == nil {
			b.Fatalf("saveNull readback: %v", err)
		}
		if p.Nickname != nil || p.Sex != nil || p.Vip != nil {
			b.Fatalf("saveNull: %v %v %v", p.Nickname, p.Sex, p.Vip)
		}
	}
}

func BenchmarkSaveEnum(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	nick := "enum"
	sex := GenderFemale
	vip := Vip3
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Sex travels through the DBEnum interface, Vip through a registered
		// Converter.
		key, err := Save(ctx, &Profile{UserId: 1, Nickname: &nick, Sex: &sex, Vip: &vip})
		if err != nil {
			b.Fatal(err)
		}
		p, err := Select[Profile]().ById(ctx, key.AsLong())
		if err != nil || p == nil {
			b.Fatalf("saveEnum readback: %v", err)
		}
		if p.Sex == nil || *p.Sex != GenderFemale || p.Vip == nil || *p.Vip != Vip3 {
			b.Fatalf("saveEnum: %v %v", p.Sex, p.Vip)
		}
	}
}

func BenchmarkSavePlural(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Category -> categories: the irregular English plural engine, shared
		// with the Java side.
		key, err := Save(ctx, &Category{Name: fmt.Sprintf("cat_%05d", i)})
		if err != nil {
			b.Fatal(err)
		}
		if key.AsLong() <= 0 {
			b.Fatalf("savePlural: key = %d", key.AsLong())
		}
	}
}

func BenchmarkSaveBatch50(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	users := make([]*User, 50)
	for i := range users {
		users[i] = &User{UserName: "bench", Age: 30}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := SaveBatch(ctx, users); err != nil {
			b.Fatal(err)
		}
	}
}

// ---------------------------------------------------------------------------
// transaction path
// ---------------------------------------------------------------------------

func BenchmarkAtomicSave(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := Atomic(ctx, func(ctx context.Context) error {
			_, err := Save(ctx, &User{UserName: "bench", Age: 30})
			return err
		})
		if r.Error() != nil {
			b.Fatal(r.Error())
		}
	}
}

func BenchmarkTxRollback(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := Atomic(ctx, func(ctx context.Context) error {
			if _, err := Save(ctx, &User{UserName: "rollback", Age: 31}); err != nil {
				return err
			}
			// The default rollback predicate only rolls back for
			// RuntimeException (mirroring the Java rollbackException default),
			// so the forced failure is wrapped with Unchecked.
			return Unchecked(fmt.Errorf("forced rollback"))
		})
		if r.Error() == nil {
			b.Fatal("txRollback: expected error")
		}
		if !r.IsRollback() {
			b.Fatal("txRollback: expected rollback state")
		}
	}
}

// BenchmarkTxSavepointNested covers nested Atomic (SAVEPOINT), which the Java
// side has no counterpart for.
func BenchmarkTxSavepointNested(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := Atomic(ctx, func(ctx context.Context) error {
			if _, err := Save(ctx, &User{UserName: "outer", Age: 31}); err != nil {
				return err
			}
			inner := Atomic(ctx, func(ctx context.Context) error {
				_, err := Save(ctx, &User{UserName: "inner", Age: 32})
				return err
			})
			return inner.Error()
		})
		if r.Error() != nil {
			b.Fatal(r.Error())
		}
	}
}

// ---------------------------------------------------------------------------
// read path
// ---------------------------------------------------------------------------

func BenchmarkSelectAll(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().All(ctx)
		if err != nil || len(list) != benchSeedRows {
			b.Fatalf("all: %v %d", err, len(list))
		}
	}
}

func BenchmarkById(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u, err := Select[User]().ById(ctx, 42)
		if err != nil || u == nil {
			b.Fatalf("byId: %v", err)
		}
	}
}

func BenchmarkByIds(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	ids := make([]any, 10)
	for i := range ids {
		ids[i] = i + 1
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().ByIds(ctx, ids...)
		if err != nil || len(list) != len(ids) {
			b.Fatalf("byIds: %v %d", err, len(list))
		}
	}
}

func BenchmarkOne(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		u, err := Select[User]().WhereCol("Age").Gt(50).One(ctx)
		if err != nil || u == nil {
			b.Fatalf("one: %v", err)
		}
	}
}

func BenchmarkCount(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := Select[User]().Count(ctx)
		if err != nil || n != benchSeedRows {
			b.Fatalf("count: %v %d", err, n)
		}
	}
}

func BenchmarkMaps(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rows, err := Select[User]().Maps(ctx)
		if err != nil || len(rows) != benchSeedRows {
			b.Fatalf("maps: %v %d", err, len(rows))
		}
	}
}

func BenchmarkWhereEq(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().WhereCol("Age").Eq(30).All(ctx)
		if err != nil || len(list) != 1 {
			b.Fatalf("whereEq: %v %d", err, len(list))
		}
	}
}

func BenchmarkWhereQuery(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().WhereCol("Age").Gt(25).All(ctx)
		if err != nil || len(list) != 75 { // ages 26..100 of the seeded 100 rows
			b.Fatalf("where: %v %d", err, len(list))
		}
	}
}

func BenchmarkWhereNe(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().WhereCol("Age").Ne(30).All(ctx)
		if err != nil || len(list) != 99 {
			b.Fatalf("whereNe: %v %d", err, len(list))
		}
	}
}

func BenchmarkWhereIn(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().WhereCol("Age").In(10, 20, 30, 40, 50).All(ctx)
		if err != nil || len(list) != 5 {
			b.Fatalf("whereIn: %v %d", err, len(list))
		}
	}
}

func BenchmarkWhereLike(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().WhereCol("UserName").Like("user_00%").All(ctx)
		if err != nil || len(list) != 9 { // user_001..user_009
			b.Fatalf("whereLike: %v %d", err, len(list))
		}
	}
}

func BenchmarkWhereBetween(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().WhereCol("Age").Between(20, 29).All(ctx)
		if err != nil || len(list) != 10 { // ages 20..29
			b.Fatalf("whereBetween: %v %d", err, len(list))
		}
	}
}

func BenchmarkWhereIsNull(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[Profile]().WhereCol("Nickname").IsNull().All(ctx)
		if err != nil || len(list) != 33 { // every third seeded profile row
			b.Fatalf("whereIsNull: %v %d", err, len(list))
		}
	}
}

func BenchmarkWhereNotNull(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[Profile]().WhereCol("Nickname").NotNull().All(ctx)
		if err != nil || len(list) != 67 {
			b.Fatalf("whereNotNull: %v %d", err, len(list))
		}
	}
}

func BenchmarkWhereAndOr(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().
			Where("age > ?", 20).And("age < ?", 30).Or("age > ?", 90).
			All(ctx)
		if err != nil || len(list) != 19 { // ages 21..29 plus 91..100
			b.Fatalf("whereAndOr: %v %d", err, len(list))
		}
	}
}

func BenchmarkOrderLimit(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().Order("age desc").Limit(10).All(ctx)
		if err != nil || len(list) != 10 {
			b.Fatalf("orderLimit: %v %d", err, len(list))
		}
	}
}

func BenchmarkOrderCol(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().OrderCol("Age", "desc").Limit(10).All(ctx)
		if err != nil || len(list) != 10 {
			b.Fatalf("orderCol: %v %d", err, len(list))
		}
	}
}

func BenchmarkPage(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p, err := Select[User]().Page(ctx, 5, 10)
		if err != nil {
			b.Fatalf("page: %v", err)
		}
		if p.TotalRows != int64(benchSeedRows) || len(p.Rows) != 10 {
			b.Fatalf("page: total=%d rows=%d", p.TotalRows, len(p.Rows))
		}
	}
}

func BenchmarkWhereModel(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().
			WhereModel(&User{Id: 42, UserName: "user_042", Age: 42}).
			All(ctx)
		if err != nil || len(list) != 1 {
			b.Fatalf("whereModel: %v %d", err, len(list))
		}
	}
}

func BenchmarkSelectCols(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().Cols("user_name", "age").All(ctx)
		if err != nil || len(list) != benchSeedRows {
			b.Fatalf("selectCols: %v %d", err, len(list))
		}
	}
}

func BenchmarkExcludeCols(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().Exclude("email", "created_at").All(ctx)
		if err != nil || len(list) != benchSeedRows {
			b.Fatalf("excludeCols: %v %d", err, len(list))
		}
	}
}

// Associations are back-filled row by row on both sides (no JOIN SQL), so the
// cost is one extra query per loaded row.
func BenchmarkJoinHasOne(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		o, err := Select[Order]().
			Join(JoinWith[User]().As("Owner").On("Uid", "Id")).
			ById(ctx, 42)
		if err != nil || o == nil {
			b.Fatalf("joinHasOne: %v", err)
		}
		if o.Owner == nil {
			b.Fatal("joinHasOne: owner not loaded")
		}
	}
}

func BenchmarkJoinHasMany(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().
			WhereCol("Id").Le(10).
			Join(JoinWith[Order]().As("Orders").On("Id", "Uid").Order("id desc")).
			All(ctx)
		if err != nil || len(list) != 10 {
			b.Fatalf("joinHasMany: %v %d", err, len(list))
		}
		total := 0
		for _, u := range list {
			total += len(u.Orders)
		}
		if total != 50 { // ten users, five orders each
			b.Fatalf("joinHasMany: %d orders", total)
		}
	}
}

func BenchmarkBySQL(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list, err := Select[User]().BySQL("SELECT * FROM users WHERE age > ?", 50).All(ctx)
		if err != nil || len(list) != 50 {
			b.Fatalf("bySQL: %v %d", err, len(list))
		}
	}
}

func BenchmarkScalarSQL(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := ScalarSQL[int](ctx, "SELECT COUNT(*) FROM users WHERE age > ?", 50)
		if err != nil || n != 50 {
			b.Fatalf("scalarSQL: %v %d", err, n)
		}
	}
}

func BenchmarkExecuteSQL(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		res, err := Execute(ctx, "UPDATE users SET age = age + 1 WHERE id = ?", i%benchSeedRows+1)
		if err != nil {
			b.Fatal(err)
		}
		n, err := res.RowsAffected()
		if err != nil || n != 1 {
			b.Fatalf("executeSQL: %v %d", err, n)
		}
	}
}

// ---------------------------------------------------------------------------
// update path
// ---------------------------------------------------------------------------

func BenchmarkUpdateById(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// The value rotates because MySQL reports only actually-changed rows:
		// re-writing the same value leaves affected=0 from the second
		// iteration on. The Java harness rotates identically.
		n, err := UpdateFrom[User]().
			Set("UserName", fmt.Sprintf("bench%04d", i%10000)).
			UpdateById(ctx, 42)
		if err != nil || n != 1 {
			b.Fatalf("updateById: %v %d", err, n)
		}
	}
}

func BenchmarkUpdateByWhere(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := UpdateFrom[User]().
			Set("UserName", fmt.Sprintf("bench%04d", i%10000)).
			WhereCol("Age").Lt(20).
			Execute(ctx)
		if err != nil || n != 19 { // seeded ages 1..19
			b.Fatalf("updateByWhere: %v %d", err, n)
		}
	}
}

func BenchmarkUpdateModel(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := Update(ctx, &Category{Id: 7, Name: fmt.Sprintf("cat_%05d", i)})
		if err != nil || n != 1 {
			b.Fatalf("updateModel: %v %d", err, n)
		}
	}
}

// ---------------------------------------------------------------------------
// delete path
// ---------------------------------------------------------------------------

// benchResetDeletePool refills delete_targets so each delete case starts from
// a full pool regardless of what earlier runs (in this or another process)
// consumed. It runs before the timer, so the fill cost is never measured.
func benchResetDeletePool(b *testing.B) {
	b.Helper()
	ctx := context.Background()
	if _, err := Execute(ctx, "DELETE FROM delete_targets"); err != nil {
		b.Fatal(err)
	}
	const batch = 5000
	for start := 1; start <= benchDeletePool; start += batch {
		end := start + batch - 1
		if end > benchDeletePool {
			end = benchDeletePool
		}
		var sb strings.Builder
		sb.WriteString("INSERT INTO delete_targets (id, name) VALUES ")
		for i := start; i <= end; i++ {
			if i > start {
				sb.WriteString(", ")
			}
			fmt.Fprintf(&sb, "(%d, 'dt_%05d')", i, i)
		}
		if _, err := Execute(ctx, sb.String()); err != nil {
			b.Fatal(err)
		}
	}
}

// benchUndoDeleteTargets re-inserts the rows a delete case just removed, with
// the timer stopped. The delete pool therefore stays full no matter how many
// iterations a side runs, so both harnesses measure a steady state instead of
// one draining the table. StartTimer re-bases the allocation counters too, so
// the restore's own allocations stay out of allocs/op.
func benchUndoDeleteTargets(b *testing.B, ids ...int64) {
	b.Helper()
	b.StopTimer()
	var sb strings.Builder
	sb.WriteString("INSERT INTO delete_targets (id, name) VALUES ")
	for i, id := range ids {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "(%d, 'dt_%05d')", id, id)
	}
	if _, err := Execute(context.Background(), sb.String()); err != nil {
		b.Fatal(err)
	}
	b.StartTimer()
}

func BenchmarkDeleteById(b *testing.B) {
	benchInstance(b)
	benchResetDeletePool(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seq := benchNextDeleteID(&benchSeqDeleteById, 1)
		n, err := DeleteById[DeleteTarget](ctx, seq)
		if err != nil {
			b.Fatal(err)
		}
		if n != 1 {
			b.Fatalf("deleteById: %d", n)
		}
		benchUndoDeleteTargets(b, seq)
	}
}

func BenchmarkDeleteModel(b *testing.B) {
	benchInstance(b)
	benchResetDeletePool(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seq := benchNextDeleteID(&benchSeqDeleteModel, 1)
		n, err := Delete(ctx, &DeleteTarget{Id: seq, Name: fmt.Sprintf("dt_%05d", seq)})
		if err != nil {
			b.Fatal(err)
		}
		if n != 1 {
			b.Fatalf("deleteModel: %d", n)
		}
		benchUndoDeleteTargets(b, seq)
	}
}

func BenchmarkDeleteBatch(b *testing.B) {
	benchInstance(b)
	benchResetDeletePool(b)
	ctx := context.Background()
	ids := make([]any, 10)
	restored := make([]int64, len(ids))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		base := benchNextDeleteID(&benchSeqDeleteBatch, int64(len(ids)))
		for j := range ids {
			restored[j] = base + int64(j)
			ids[j] = restored[j]
		}
		n, err := DeleteBatch[DeleteTarget](ctx, ids...)
		if err != nil {
			b.Fatal(err)
		}
		if n != 10 {
			b.Fatalf("deleteBatch: %d", n)
		}
		benchUndoDeleteTargets(b, restored...)
	}
}

// ---------------------------------------------------------------------------
// in-memory Page utilities (no database round-trip)
// ---------------------------------------------------------------------------

func BenchmarkMapPage(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	page, err := Select[User]().Page(ctx, 1, benchSeedRows)
	if err != nil {
		b.Fatal(err)
	}
	if page.TotalRows != int64(benchSeedRows) || len(page.Rows) != benchSeedRows {
		b.Fatalf("mapPage setup: total=%d rows=%d", page.TotalRows, len(page.Rows))
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mapped := MapPage(page, func(u *User) int { return u.Age })
		if len(mapped.Rows) != benchSeedRows {
			b.Fatalf("mapPage: %d rows", len(mapped.Rows))
		}
	}
}

func BenchmarkPeekPage(b *testing.B) {
	benchInstance(b)
	benchReset(b)
	ctx := context.Background()
	page, err := Select[User]().Page(ctx, 1, benchSeedRows)
	if err != nil {
		b.Fatal(err)
	}
	if page.TotalRows != int64(benchSeedRows) || len(page.Rows) != benchSeedRows {
		b.Fatalf("peekPage setup: total=%d rows=%d", page.TotalRows, len(page.Rows))
	}
	sum := 0
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page.Peek(func(u *User) { sum += u.Age })
	}
	if sum == 0 {
		b.Fatal("peekPage: no rows visited")
	}
}
