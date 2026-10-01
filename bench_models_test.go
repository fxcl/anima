package anima

import (
	"reflect"
	"strconv"
)

// Models backing the full-coverage benchmark suite. They mirror the Java
// harness entities one-to-one and rely on the same shared benchserver schema
// (see benchserver/main.go).

// Category exercises the irregular English plural engine: the class pluralizes
// to "categories" rather than "categorys", identically on both sides.
type Category struct {
	Id   int64
	Name string
}

// Order is the join association target: users 1..20 each own exactly five
// orders in the seeded data. Owner is the hasOne/belongsTo mount field.
type Order struct {
	Id          int64
	Uid         int
	ProductName string
	Owner       *User `anima:"-"`
}

// BenchUser exercises TablePrefix: with prefix "bench" it maps to
// bench_bench_users on both sides (plural(prefix + "_" + toUnderline(name))).
type BenchUser struct {
	Id       int64
	UserName string
	Age      int
}

// Profile mixes NULL-able columns with enum columns. Sex goes through the
// DBEnum interface, Vip through the converter registry.
type Profile struct {
	Id       int64
	UserId   int
	Nickname *string
	Sex      *Gender
	Vip      *VipLevel
}

// DeleteTarget backs the delete benchmarks. Its table holds a large row pool
// so every iteration deletes a row that has not been deleted before, keeping
// the affected-row count deterministic.
type DeleteTarget struct {
	Id   int64
	Name string
}

// Gender is the DBEnum side of the enum coverage (the @EnumMapping equivalent).
type Gender int

const (
	GenderUnknown Gender = iota
	GenderMale
	GenderFemale
)

func (g Gender) ToDatabaseValue() any { return int(g) }

func (g *Gender) SetDatabaseValue(v any) error {
	n, err := benchToInt64(v)
	if err != nil {
		return err
	}
	*g = Gender(n)
	return nil
}

// VipLevel goes through the converter registry (the Converter equivalent).
type VipLevel int

const (
	VipNone VipLevel = iota
	Vip1
	Vip2
	Vip3
)

type vipConverter struct{}

func (vipConverter) Target() reflect.Type { return reflect.TypeOf(VipLevel(0)) }

func (vipConverter) ToDatabase(v any) (any, error) {
	return int64(v.(VipLevel)), nil
}

func (vipConverter) FromDatabase(v any) (any, error) {
	n, err := benchToInt64(v)
	if err != nil {
		return nil, err
	}
	return VipLevel(n), nil
}

func init() {
	AddConverter(vipConverter{})
}

// benchToInt64 accepts the scalar shapes database/sql hands back for integer
// columns.
func benchToInt64(v any) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case []byte:
		return strconv.ParseInt(string(n), 10, 64)
	case string:
		return strconv.ParseInt(n, 10, 64)
	default:
		return 0, newError(ErrorCodeMappingFailure, "cannot convert %T to int64", v)
	}
}
