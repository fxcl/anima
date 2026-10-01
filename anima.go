// Package anima is a Go ORM with no code generation, convention over
// configuration, and minimal struct tags: a fluent query DSL, pluggable SQL
// dialects, and Atomic transactions that return an observable result instead
// of panicking.
//
// Key design choices:
//   - ActiveRecord-style methods become generic free functions such as
//     anima.Save(ctx, &u), because Go embedding cannot give a base method
//     access to the outer entity.
//   - Transaction connections propagate through context.Context explicitly.
//   - Field references are plain strings, validated against entity metadata
//     at build time (unknown fields are recorded and surface at execution).
package anima

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/fxcl/anima/dialect"
)

// Anima is the global entry point holding the connection pool, the SQL
// dialect, and global configuration.
type Anima struct {
	db           *sql.DB
	tablePrefix  string
	dialect      dialect.Dialect
	useSQLLimit  bool
	sqlStatistic bool
	rollbackWhen func(error) bool
}

var (
	mu           sync.RWMutex
	defaultAnima *Anima
)

// GetTablePrefix returns the current table prefix (Java tablePrefix() getter).
func (a *Anima) GetTablePrefix() string { return a.tablePrefix }

// GetDialect returns the current dialect (Java dialect() getter).
func (a *Anima) GetDialect() dialect.Dialect { return a.dialect }

// IsUseSQLLimit reports whether LIMIT placeholders are enabled (Java isUseSQLLimit()).
func (a *Anima) IsUseSQLLimit() bool { return a.useSQLLimit }

// IsEnableSQLStatistic reports whether SQL statistics are enabled (Java isEnableSQLStatistic()).
func (a *Anima) IsEnableSQLStatistic() bool { return a.sqlStatistic }

type txKey struct{}

// Open opens a DSN with the MySQL driver and registers it as the global
// default instance.
func Open(dsn string) (*Anima, error) {
	return OpenWith("mysql", dsn)
}

// OpenWith opens a DSN with the given driver name (SQLite, PostgreSQL, etc.).
func OpenWith(driverName, dsn string) (*Anima, error) {
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	a := &Anima{db: db, dialect: dialect.MySQL{}, useSQLLimit: true, sqlStatistic: true, rollbackWhen: defaultRollbackWhen}
	SetDefault(a)
	return a, nil
}

// OpenDB takes over an already established *sql.DB.
func OpenDB(db *sql.DB) *Anima {
	a := &Anima{db: db, dialect: dialect.MySQL{}, useSQLLimit: true, sqlStatistic: true, rollbackWhen: defaultRollbackWhen}
	SetDefault(a)
	return a
}

// SetDefault sets the global default instance.
func SetDefault(a *Anima) {
	mu.Lock()
	defer mu.Unlock()
	defaultAnima = a
}

// Of returns the global default instance.
func Of() *Anima {
	mu.RLock()
	defer mu.RUnlock()
	return defaultAnima
}

// TablePrefix sets the global table prefix (User with prefix "t" -> t_users).
func (a *Anima) TablePrefix(prefix string) *Anima {
	a.tablePrefix = prefix
	return a
}

// Dialect sets the SQL generation dialect.
func (a *Anima) Dialect(d dialect.Dialect) *Anima {
	a.dialect = d
	return a
}

// UseSQLLimit controls whether limit is rendered into SQL or truncated in
// memory after fetching.
func (a *Anima) UseSQLLimit(v bool) *Anima {
	a.useSQLLimit = v
	return a
}

// RollbackWhen sets the transaction rollback predicate.
// Pass nil to restore the default (only errors implementing RuntimeException
// or runtime.Error trigger rollback).
func (a *Anima) RollbackWhen(f func(error) bool) *Anima {
	if f == nil {
		a.rollbackWhen = defaultRollbackWhen
	} else {
		a.rollbackWhen = f
	}
	return a
}

// EnableSQLStatistic toggles SQL execution statistic logging (on by default).
// Statistics are decided at the anima execution exit; the underlying driver
// never reads the global singleton back.
func (a *Anima) EnableSQLStatistic(v bool) *Anima {
	a.sqlStatistic = v
	return a
}

// SetPool configures the underlying *sql.DB connection pool. maxOpen caps
// concurrent connections (<=0 keeps database/sql's unlimited default);
// maxIdle caps connections kept between bursts (<=0 keeps the default of 2,
// which forces every spare concurrent query to tear down and re-dial —
// under any real concurrency set maxIdle to at least your worker count).
// database/sql's Set* methods are safe to call on a live pool, so this may
// be re-tuned at runtime. Returns the receiver for chaining.
func (a *Anima) SetPool(maxOpen, maxIdle int) *Anima {
	a.db.SetMaxOpenConns(maxOpen)
	a.db.SetMaxIdleConns(maxIdle)
	return a
}

// execer is the common statement execution surface of database/sql
// (implemented by both *sql.DB and *sql.Tx).
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// Begin opens a manual transaction and returns a ctx that routes every
// statement made with it -- saves, queries, and nested Atomic blocks (which
// degrade to SAVEPOINTs) -- through that connection until Commit or Rollback.
func (a *Anima) Begin(ctx context.Context) (context.Context, error) {
	if a.db == nil {
		return ctx, errNotInit()
	}
	t, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, txKey{}, t), nil
}

// Commit finalizes the manual transaction carried in ctx.
func (a *Anima) Commit(ctx context.Context) error {
	t, ok := ctx.Value(txKey{}).(*sql.Tx)
	if !ok {
		return newError(ErrorCodeInvalidArgument, "Commit: no manual transaction in ctx")
	}
	return t.Commit()
}

// Rollback abandons the manual transaction carried in ctx. Safe to call as a
// defer alongside Commit: a second call on the finished transaction surfaces
// database/sql's own "already committed" error.
func (a *Anima) Rollback(ctx context.Context) error {
	t, ok := ctx.Value(txKey{}).(*sql.Tx)
	if !ok {
		return newError(ErrorCodeInvalidArgument, "Rollback: no manual transaction in ctx")
	}
	return t.Rollback()
}

// sqlable prefers the transaction connection carried in ctx, falling back to
// the connection pool.
func (a *Anima) sqlable(ctx context.Context) (execer, error) {
	if tx, ok := ctx.Value(txKey{}).(*sql.Tx); ok {
		return tx, nil
	}
	if a.db == nil {
		return nil, errNotInit()
	}
	return a.db, nil
}

// execCtx/queryCtx are the single exit for every statement execution,
// performing write-path parameter binding (converter/DBEnum) and SQL
// statistic logging in one place.
func (a *Anima) execCtx(ctx context.Context, ex execer, sqlText string, args []any) (sql.Result, error) {
	bound, err := bindParams(args)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	res, err := ex.ExecContext(ctx, sqlText, bound...)
	if a.sqlStatistic {
		log.Printf("anima: %v %s | err=%v", time.Since(start), sqlText, err)
	}
	return res, err
}

func (a *Anima) queryCtx(ctx context.Context, ex execer, sqlText string, args []any) (*sql.Rows, error) {
	bound, err := bindParams(args)
	if err != nil {
		return nil, err
	}
	start := time.Now()
	rows, err := ex.QueryContext(ctx, sqlText, bound...)
	if a.sqlStatistic {
		log.Printf("anima: %v %s | err=%v", time.Since(start), sqlText, err)
	}
	return rows, err
}
