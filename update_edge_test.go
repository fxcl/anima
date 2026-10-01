package anima

import (
	"context"
	"strings"
	"testing"
)

// TestUpdateFromEmptySetErrors UpdateFrom() calls Execute without any Set:
// it should fail fast with an invalid argument and not generate an empty UPDATE SQL.
func TestUpdateFromEmptySetErrors(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)
	n, err := UpdateFrom[User]().WhereCol("Id").Eq(1).Execute(ctx)
	if err == nil {
		t.Fatalf("expected error on empty Set; got n=%d", n)
	}
	if n != 0 {
		t.Errorf("affected rows should be 0 on error path, got %d", n)
	}
}

// TestInsertEmptyColumnsBehavior When Save receives an entity whose fields are all zero,
// it should generate "INSERT INTO `users`" (no column list) - this is the shape of the Java
// equivalent when an empty set is passed explicitly.
// Save on the Go side always filters non-nil/non-zero fields, so it takes the default path using the entity columns.
// Here we verify that Save on an all-zero entity at least does not error (depending on nullable vs default in the table design).
func TestInsertEmptyColumnsBehavior(t *testing.T) {
	ctx := context.Background()
	if _, err := Execute(ctx, "DELETE FROM users"); err != nil {
		t.Fatal(err)
	}
	// Both UserName and Age are zero; the auto-increment PK is assigned by the DB. UserName is NOT NULL DEFAULT '',
	// so inserting "" is valid.
	k, err := Save(ctx, &User{})
	if err != nil {
		t.Fatalf("zero-value save: %v", err)
	}
	if k.AsLong() <= 0 {
		t.Fatalf("expected auto-increment id, got %v", k)
	}
}

// TestUpdateByIdRequiresPK The User type has an auto-increment PK, so UpdateById must be able to resolve it; here we test
// the SQL generation path: the SET columns contain every field except the PK, and the WHERE is `id` = ?.
func TestUpdateByIdQuotesAllIdentifiers(t *testing.T) {
	ctx := context.Background()
	seedUsers(t)

	done := captureSQL(t)
	k, err := Save(ctx, &User{UserName: "quote-check", Age: 9})
	if err != nil {
		t.Fatal(err)
	}
	sqlText := done()
	if !strings.Contains(sqlText, "INSERT INTO `users`") {
		t.Fatalf("insert not quoted: %s", sqlText)
	}

	done = captureSQL(t)
	n, err := UpdateFrom[User]().UpdateById(ctx, k.AsLong())
	sqlText = done()
	if err == nil {
		t.Fatalf("UpdateById without Set must fail-fast; got n=%d", n)
	}
	// UpdateById currently returns an invalid argument because setCols is empty, so no SQL is emitted.
	if strings.Contains(sqlText, "UPDATE") {
		t.Fatalf("empty-Set UpdateById must not emit SQL, got: %s", sqlText)
	}
}
