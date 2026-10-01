package anima

import (
	"math/big"
	"strconv"
)

// ResultKey wraps the auto-increment primary key returned by an insert.
type ResultKey struct {
	key int64
}

func (r *ResultKey) AsLong() int64 { return r.key }

func (r *ResultKey) AsInt() int { return int(r.key) }

// AsBigInteger returns the key as an arbitrary-precision integer. Java's
// ResultKey.asBigInteger casts a BigInteger handed back by certain JDBC
// drivers; database/sql always yields an int64, so the conversion is the
// semantic equivalent ("primary key as big integer").
func (r *ResultKey) AsBigInteger() *big.Int { return big.NewInt(r.key) }

// AsString returns the key in decimal string form.
func (r *ResultKey) AsString() string { return strconv.FormatInt(r.key, 10) }

func (r *ResultKey) String() string { return strconv.FormatInt(r.key, 10) }
