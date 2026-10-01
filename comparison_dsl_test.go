package anima

import "testing"

// Covers the comparison DSL on the read path: Ne, Ge, Gt, Lt, Le and Like had
// zero coverage (they were only exercised through the update path for Lt).
func TestSelectComparisonDSL(t *testing.T) {
	ctx := t.Context()
	seedUsers(t)

	us, err := Select[User]().WhereCol("UserName").Ne("ann").All(ctx)
	if err != nil || len(us) != 2 {
		t.Fatalf("ne: %v %+v", err, us)
	}

	us, err = Select[User]().WhereCol("Age").Ge(20).All(ctx)
	if err != nil || len(us) != 2 {
		t.Fatalf("ge: %v %+v", err, us)
	}

	us, err = Select[User]().WhereCol("Age").Gt(20).All(ctx)
	if err != nil || len(us) != 1 || us[0].UserName != "ann" {
		t.Fatalf("gt: %v %+v", err, us)
	}

	us, err = Select[User]().WhereCol("Age").Lt(20).All(ctx)
	if err != nil || len(us) != 1 || us[0].UserName != "cat" {
		t.Fatalf("lt: %v %+v", err, us)
	}

	us, err = Select[User]().WhereCol("Age").Le(20).All(ctx)
	if err != nil || len(us) != 2 {
		t.Fatalf("le: %v %+v", err, us)
	}

	us, err = Select[User]().WhereCol("UserName").Like("a%").All(ctx)
	if err != nil || len(us) != 1 || us[0].UserName != "ann" {
		t.Fatalf("like: %v %+v", err, us)
	}
}
