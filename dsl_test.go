package anima

import (
	"context"
	"testing"
)

// seedUsers clears the table and inserts three rows, returning their ids.
func seedUsers(t *testing.T) []int64 {
	t.Helper()
	ctx := context.Background()
	if _, err := Execute(ctx, "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	names := []struct {
		name string
		age  int
	}{{"ann", 30}, {"bob", 20}, {"cat", 10}}
	ids := make([]int64, 0, 3)
	for _, n := range names {
		k, err := Save(ctx, &User{UserName: n.name, Age: n.age})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, k.AsLong())
	}
	return ids
}

func TestUpdateDSL(t *testing.T) {
	ctx := context.Background()
	ids := seedUsers(t)

	// Alignment guard for update().from(User.class).set(User::getUserName, "base64").updateById(2)
	n, err := UpdateFrom[User]().Set("UserName", "base64").UpdateById(ctx, ids[1])
	if err != nil || n != 1 {
		t.Fatalf("updateById: %v %d", err, n)
	}
	u, err := Select[User]().ById(ctx, ids[1])
	if err != nil || u == nil || u.UserName != "base64" {
		t.Fatalf("got %+v", u)
	}

	// Conditional update: update().from(...).where(User::getAge).lt(15).execute()
	n, err = UpdateFrom[User]().Set("Age", 1).WhereCol("Age").Lt(15).Execute(ctx)
	if err != nil || n != 1 {
		t.Fatalf("conditional update: %v %d", err, n)
	}
}

func TestUpdateModel(t *testing.T) {
	ctx := context.Background()
	ids := seedUsers(t)

	u, err := Select[User]().ById(ctx, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	u.Age = 31
	n, err := Update(ctx, u) // Alignment guard for Java model.update(): null fields skipped
	if err != nil || n != 1 {
		t.Fatalf("model update: %v %d", err, n)
	}
	back, _ := Select[User]().ById(ctx, ids[0])
	if back.Age != 31 {
		t.Fatalf("age = %d", back.Age)
	}
}

func TestDeleteDSL(t *testing.T) {
	ctx := context.Background()
	ids := seedUsers(t)

	// delete().from(User.class).where(User::getAge).lte(20).execute()
	n, err := DeleteFrom[User]().WhereCol("Age").Le(20).Execute(ctx)
	if err != nil || n != 2 { // bob(20) and cat(10)
		t.Fatalf("delete: %v %d", err, n)
	}

	// Anima.deleteById(User.class, id)
	n, err = DeleteById[User](ctx, ids[0])
	if err != nil || n != 1 {
		t.Fatalf("deleteById: %v %d", err, n)
	}

	rest, _ := Select[User]().All(ctx)
	if len(rest) != 0 {
		t.Fatalf("remaining %+v", rest)
	}
}

func TestDeleteModel(t *testing.T) {
	ctx := context.Background()
	ids := seedUsers(t)
	u, _ := Select[User]().ById(ctx, ids[2])
	n, err := Delete(ctx, u) // Delete by the entity's non-empty fields
	if err != nil || n != 1 {
		t.Fatalf("model delete: %v %d", err, n)
	}
	cnt, _ := Select[User]().Count(ctx)
	if cnt != 2 {
		t.Fatalf("count = %d", cnt)
	}
}

func TestColsOrderLimit(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	// Alignment guard for select(User::getUserName, User::getAge).from(User.class).order("id desc")
	list, err := Select[User]().Cols("UserName", "Age").Order("id desc").All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].UserName != "cat" {
		t.Fatalf("got %+v", list)
	}
	if list[0].Email != nil {
		t.Fatal("unselected column must stay zero")
	}

	list, err = Select[User]().Limit(2).OrderCol("Id", "asc").All(ctx)
	if err != nil || len(list) != 2 || list[0].UserName != "ann" {
		t.Fatalf("limit: %v %+v", err, list)
	}
}

func TestBySQLAndScalarChannels(t *testing.T) {
	ctx := context.Background()
	ids := seedUsers(t)

	// Raw SQL to entities (select().bySQL(User.class, sql))
	list, err := Select[User]().BySQL("select * from users where age > ?", 15).All(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("bySQL: %v %d", err, len(list))
	}

	// Raw SQL to a scalar column (select().bySQL(String.class, sql))
	names, err := QuerySQL[string](ctx, "select user_name from users where id < ? order by id", ids[1])
	if err != nil || len(names) != 1 || names[0] != "ann" {
		t.Fatalf("QuerySQL: %v %+v", err, names)
	}

	// executeScalar (count)
	total, err := ScalarSQL[int](ctx, "select count(*) from users")
	if err != nil || total != 3 {
		t.Fatalf("ScalarSQL: %v %d", err, total)
	}
}

func TestInAndNullConditions(t *testing.T) {
	ctx := context.Background()
	ids := seedUsers(t)
	if _, err := Execute(ctx, "UPDATE users SET email = ? WHERE id = ?", "x@y.z", ids[0]); err != nil {
		t.Fatal(err)
	}

	list, err := Select[User]().WhereCol("Id").In(ids[0], ids[2]).All(ctx)
	if err != nil || len(list) != 2 {
		t.Fatalf("in: %v %+v", err, list)
	}

	list, err = Select[User]().WhereCol("Email").IsNull().All(ctx)
	if err != nil || len(list) != 2 { // only ids[0] has a non-null email
		t.Fatalf("isNull: %v %+v", err, list)
	}

	list, err = Select[User]().WhereCol("Email").NotNull().All(ctx)
	if err != nil || len(list) != 1 || list[0].Id != ids[0] {
		t.Fatalf("notNull: %v %+v", err, list)
	}
}
