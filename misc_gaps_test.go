package anima

import (
	"reflect"
	"testing"
)

func TestToInt64Kinds(t *testing.T) {
	i64 := int64(7)
	cases := []struct {
		name string
		v    any
		want int64
		ok   bool
	}{
		{"int", 7, 7, true},
		{"int64", int64(7), 7, true},
		{"int8", int8(7), 7, true},
		{"uint32", uint32(7), 7, true},
		{"ptr-int64", &i64, 7, true},
		{"nil-ptr", (*int64)(nil), 0, false},
		{"string", "7", 0, false},
	}
	for _, c := range cases {
		got, ok := toInt64(reflect.ValueOf(c.v))
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s: got (%d,%v), want (%d,%v)", c.name, got, ok, c.want, c.ok)
		}
	}
}

func TestAnimaMethodAddConverter(t *testing.T) {
	mu.RLock()
	a := defaultAnima
	mu.RUnlock()
	if got := a.AddConverter(statusConverter{}); got != a {
		t.Fatal("(*Anima).AddConverter must return the receiver for chaining")
	}
}
