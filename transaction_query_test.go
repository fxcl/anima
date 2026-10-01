package anima

import (
	"errors"
	"testing"
)

func TestUncheckedSemantics(t *testing.T) {
	if Unchecked(nil) != nil {
		t.Fatal("Unchecked(nil) must stay nil")
	}
	already := uncheckedErr{err: errors.New("x")}
	if Unchecked(already) != error(already) {
		t.Fatal("double-wrap must be identity")
	}
	plain := errors.New("boom")
	wrapped := Unchecked(plain)
	if !defaultRollbackWhen(wrapped) {
		t.Fatal("unchecked error must trip the rollback predicate")
	}
	if defaultRollbackWhen(plain) {
		t.Fatal("plain error must NOT trip the rollback predicate")
	}
	if !errors.Is(wrapped, plain) {
		t.Fatal("unwrap chain through uncheckedErr broken")
	}
}

func TestNewQueryEntityKinds(t *testing.T) {
	ctx := t.Context()

	if _, err := Select[any]().All(ctx); err == nil {
		t.Fatal("interface entity must fail with cannot resolve entity type")
	}
	if _, err := Select[int]().All(ctx); err == nil {
		t.Fatal("non-struct entity must fail with is not a struct entity")
	}
	if _, err := Save(ctx, &User{UserName: "ptrq", Age: 9}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	us, err := Select[*User]().All(ctx)
	if err != nil {
		t.Fatalf("pointer entity should behave like its element type: %v", err)
	}
	if len(us) < 1 {
		t.Fatalf("pointer entity query returned %d rows, want >=1", len(us))
	}
}
