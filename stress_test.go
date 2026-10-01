package anima

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
)

// Concurrency stress harness. The serial suite in bench_test.go measures
// per-operation cost; these cases run the same operations from every CPU at
// once, where allocation volume turns into GC work shared by all goroutines
// and where any shared mutable state in the ORM would surface as a race.
//
// Run: go test -run='^$' -bench='BenchmarkStress' -benchmem ./anima/
// Prerequisite: the benchserver process must be listening on 127.0.0.1:33099.

// stressPool widens the connection pool: database/sql's default MaxIdleConns
// of 2 makes every spare concurrent query tear down and re-dial its
// connection, which both distorts latency and exhausts the test server's
// connection budget (observed at ~1024 cumulative connections).
func stressPool(b *testing.B) {
	benchInstance(b)
	mu.RLock()
	db := defaultAnima.db
	mu.RUnlock()
	db.SetMaxIdleConns(32)
	db.SetMaxOpenConns(64)
}

// BenchmarkStressReadOnly hammers the read path, which is where the ORM's
// per-row allocation used to concentrate.
func BenchmarkStressReadOnly(b *testing.B) {
	stressPool(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			list, err := Select[User]().All(ctx)
			if err != nil {
				b.Errorf("select: %v", err)
				return
			}
			if len(list) != benchSeedRows {
				b.Errorf("select returned %d rows, want %d", len(list), benchSeedRows)
				return
			}
		}
	})
}

// BenchmarkStressMixed interleaves the read path with single-row writes so the
// load also exercises the write path and its parameter binding.
func BenchmarkStressMixed(b *testing.B) {
	stressPool(b)
	benchReset(b)
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()

	var seq atomic.Int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := seq.Add(1)

			list, err := Select[User]().All(ctx)
			if err != nil {
				b.Errorf("select: %v", err)
				return
			}
			if len(list) != benchSeedRows {
				b.Errorf("select returned %d rows, want %d", len(list), benchSeedRows)
				return
			}

			// The value rotates on every iteration so the row really changes
			// and the write is never a no-op.
			id := int64(n%benchSeedRows) + 1
			if _, err := UpdateFrom[User]().
				Set("UserName", fmt.Sprintf("stress%08d", n)).
				UpdateById(ctx, id); err != nil {
				b.Errorf("update: %v", err)
				return
			}
		}
	})
}
