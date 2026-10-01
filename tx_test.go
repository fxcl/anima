package anima

import (
	"context"
	"errors"
	"testing"
)

func TestNestedAtomicRecoversPanic(t *testing.T) {
	ctx := t.Context()
	ar := Atomic(ctx, func(ctx context.Context) error {
		inner := Atomic(ctx, func(ctx context.Context) error {
			panic("inner boom")
		})
		if err := inner.err; err == nil {
			t.Fatal("inner atomic must surface the recovered panic")
		}
		return errors.New("outer aborts after inner panic")
	})
	if ar == nil || ar.err == nil {
		t.Fatal("outer atomic must fail after an inner panic")
	}
}
