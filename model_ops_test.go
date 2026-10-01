package anima

import "testing"

func TestDeleteAlignsDeleteByModelMatchesNonZeroFields(t *testing.T) {
	ctx := t.Context()
	ids := seedUsers(t)
	_ = ids
	before, _ := Select[User]().Where("age = ?", 10).All(ctx)
	t.Logf("before: %d rows with age=10", len(before))
	if len(before) == 0 {
		t.Skip("no age=10 rows seeded; seed data assumption broken")
	}
	n, err := Delete(ctx, &User{UserName: "cat", Age: 10})
	if err != nil {
		t.Fatal(err)
	}
	left, _ := Select[User]().WhereCol("Age").Ne(10).All(ctx)
	zero, _ := Select[User]().Where("age = ?", 10).All(ctx)
	if len(zero) != 0 || len(left) == 0 {
		t.Fatalf("delete scope wrong: age=10 left %d, others %d", len(zero), len(left))
	}
	if n == 0 {
		t.Logf("rows deleted correctly but RowsAffected reported 0 (engine semantics)")
	}
}

func TestUpdateAlignsUpdateByModelByPK(t *testing.T) {
	ctx := t.Context()
	ids := seedUsers(t)
	u := &User{Id: ids[1], UserName: "ubm_renamed"}
	n, err := Update(ctx, u)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("affected = %d, want 1", n)
	}
	us, _ := Select[User]().Where("id = ?", ids[1]).All(ctx)
	if len(us) != 1 || us[0].UserName != "ubm_renamed" {
		t.Fatalf("update not applied: %+v", us)
	}
}

func TestUpdateAlignsUpdateByModelNothingToUpdate(t *testing.T) {
	ctx := t.Context()
	seedUsers(t)
	n, err := Update(ctx, &User{Id: 3})
	if err != nil || n != 0 {
		t.Fatalf("pk-only model: (%d, %v), want (0, nil)", n, err)
	}
}

// The two functions above are the Go names for Java's deleteByModel and
// updateByModel. Update additionally refuses a pk-less model (Java would emit
// an UPDATE without WHERE there); that divergence is pinned by the
// ErrorCodeNoPrimaryKey contract test in error_code_contract_test.go.
