# anima

`anima` is the Go port of the Java ORM [anima](https://github.com/HelloKaton/anima)
(`com.hellokaton.anima`): zero codegen, convention over configuration, runtime
reflection. The module lives at `github.com/fxcl/anima`: the repository root is the importable package.

```go
import "github.com/fxcl/anima"

anima.Open("root@tcp(127.0.0.1:3306)/demo?parseTime=true")
```

Open once, then use the static-style DSL anywhere — no session objects, no generated code.

## Entities are plain structs

Conventions mirror the Java version: table = snake_case(type) pluralized
(`User` → `users`, `OrderInfo` → `order_infoes`… override with `@Table`-equivalent),
column = snake_case(field), PK = `id`.

```go
type User struct {
	Id        int64      // default primary key
	UserName  string     // -> user_name
	Age       int
	Email     *string    // NULL-able
	CreatedAt *time.Time
	Orders    []*Order   `anima:"-"` // join mount point (Java @Ignore)
}

// override table name (Java @Table(name = "order_info"))
func (OrderInfo) TableName() string { return "order_info" }
```

Tags: `anima:"column:productname"` renames a column, `anima:"-"` ignores the
field (and marks a join mount point), `anima:"pk"` designates a non-`id` PK.

## CRUD — three styles, like Java anima

```go
// ActiveRecord (Java: model.save(); Go generics replace base-class mixing)
key, _ := anima.Save(ctx, &User{UserName: "jack", Age: 25})
user.Email = &e; anima.Update(ctx, &user)   // NULL fields skipped
anima.Delete(ctx, &user)

// static helpers
anima.SaveBatch(ctx, []*User{...})
anima.DeleteById[User](ctx, 1)
anima.Execute(ctx, "create table hello_world(int integer not null)")

// SQL-flavored stream DSL
anima.Select[User]().WhereCol("Age").Gt(15).WhereCol("Email").NotNull().All(ctx)
anima.Select[User]().Cols("UserName", "Age").Order("id desc").One(ctx)
anima.UpdateFrom[User]().Set("UserName", "base64").UpdateById(ctx, 2)
anima.DeleteFrom[User]().WhereCol("Age").Le(20).Execute(ctx)

// raw SQL, typed results
names, _ := anima.QuerySQL[string](ctx, "select user_name from users limit ?", 3)
list, _ := anima.Select[User]().BySQL("select * from users where age > ?", 15).All(ctx)
```

Field references are strings (`"Age"`, `"user_name"` — the Go stand-in for
`User::getAge`) validated against entity metadata when the condition is built;
an unknown field makes the query fail fast on execution instead of at the database.

## Paging, joins, transactions

```go
page, _ := anima.Select[User]().WhereCol("Age").Gt(18).Page(ctx, 1, 10)
page.TotalRows, page.NavPageNums, page.IsLastPage
tiny := anima.MapPage(page, func(u *User) string { return u.UserName })

// N+1 back-fill joins (anima's deliberate choice, same as Java)
teams, _ := anima.Select[TeamFull]().
	Join(anima.JoinWith[Player]().As("Players").On("Id", "TeamId").Order("id asc")).
	Join(anima.JoinWith[Player]().As("Captain").On("CaptainId", "Id")).
	All(ctx)

// atomic(): the exception never escapes; it rides in an observable result
res := anima.Atomic(ctx, func(ctx context.Context) error {
	if _, err := anima.Save(ctx, u1); err != nil { return err }
	_, err := anima.Save(ctx, u2)
	return err
})
if res.IsRollback() {
	res.CatchException(func(err error) { log.Print(err) })
}
```

Transactions propagate through `context.Context` — the Go replacement for the
Java `ThreadLocal` connection. Every API (`Save`, `Select`, `Execute`, …) picks
up the ambient `*sql.Tx` automatically.

## Dialects, converters, enums

`anima/dialect` keeps anima's pluggable shape: `Base` generates all CRUD SQL;
each dialect overrides only `Paginate`. Ported: MySQL (default), SQLite,
PostgreSQL, Oracle, DB2, SqlServer, SqlServer2012.

```go
anima.Of().Dialect(dialect.PostgreSQL{}).TablePrefix("t")
```

- Custom types: `anima.AddConverter(myConv)` (sql2o `Converter` equivalent).
- Enums: implement `ToDatabaseValue() any` / `SetDatabaseValue(any) error`
  (the `@EnumMapping` equivalent); no annotation needed.
- NULLs: `*T` fields round-trip NULL, as does any type implementing
  `sql.Scanner` / `driver.Valuer`.

## Tests

Tests run against an in-process MySQL-protocol server
(`dolthub/go-mysql-server`), no database required: `go test ./...`.
