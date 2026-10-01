package anima

import "testing"

// Chaining DSL calls on a query whose construction already failed must
// short-circuit every step instead of panicking, and the root error must
// survive to the terminal call.
func TestDSLShortCircuitChain(t *testing.T) {
	ctx := t.Context()
	q := Select[any]()
	q.WhereCol("a").IsNull()
	q.WhereCol("b").NotNull()
	q.WhereCol("c").Between(1, 2)
	q.Exclude("d").Join()
	if _, err := q.ById(ctx, 1); err == nil {
		t.Fatal("root error must survive the DSL chain")
	}
	u := Select[User]()
	u.Set("Age", 1)
	if _, err := u.ById(ctx, 1); err == nil {
		t.Fatal("Set outside update mode must poison the query")
	}
	up := UpdateFrom[User]()
	up.Set("NoSuchField", 1)
	if _, err := up.UpdateById(ctx, 1); err == nil {
		t.Fatal("unknown Set field must fail in update mode")
	}
}

func TestParseTagForms(t *testing.T) {
	if col, pk := parseTag("column:full_name,pk"); col != "full_name" || !pk {
		t.Fatalf("composite tag: (%q, %v)", col, pk)
	}
	if col, pk := parseTag("pk"); col != "" || !pk {
		t.Fatalf("pk-only tag: (%q, %v)", col, pk)
	}
	if col, pk := parseTag("junk"); col != "" || pk {
		t.Fatalf("unrecognised tag: (%q, %v)", col, pk)
	}
	if col, pk := parseTag(""); col != "" || pk {
		t.Fatalf("empty tag: (%q, %v)", col, pk)
	}
}

func TestExcludeUnknownFieldAndJoinWithShapes(t *testing.T) {
	q := Select[User]().Exclude("NoSuchField")
	if _, err := q.All(t.Context()); err == nil {
		t.Fatal("Exclude of an unknown field must fail the query")
	}
	if JoinWith[Team]().err != nil {
		t.Fatal("JoinWith[Team] must be valid")
	}
	if JoinWith[*Team]().err != nil {
		t.Fatal("JoinWith[*Team] must dereference to a valid entity")
	}
	if JoinWith[int]().err == nil {
		t.Fatal("JoinWith of a non-struct must carry an error")
	}
}

func TestReadTerminalModeMismatch(t *testing.T) {
	ctx := t.Context()
	if _, err := UpdateFrom[User]().Count(ctx); err == nil {
		t.Fatal("Count outside select mode must fail")
	}
	if _, err := UpdateFrom[User]().Maps(ctx); err == nil {
		t.Fatal("Maps outside select mode must fail")
	}
}

func TestDSLUnknownFieldPinsQueryError(t *testing.T) {
	ctx := t.Context()
	if _, err := Select[User]().Cols("NoSuch").All(ctx); err == nil {
		t.Fatal("Cols with unknown field must poison the query")
	}
	if _, err := Select[User]().OrderCol("NoSuch", "ASC").All(ctx); err == nil {
		t.Fatal("OrderCol with unknown field must poison the query")
	}
	seedUsers(t)
	if _, err := Select[User]().OrderCol("Age", "ASC").All(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := Select[User]().OrderCol("Age", "").All(ctx); err != nil {
		t.Fatal(err)
	}
}
