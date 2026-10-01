// Package anima is a generic, reflection-once Go ORM in the spirit of the
// Java anima library: typed queries through generics, entity metadata computed
// exactly once, and a single funnel for every statement.
//
// # Layers
//
// The package is organised as seven strictly ordered layers; dependencies
// point one way only, from top to bottom:
//
//	anima.go      process-wide state (Anima singleton) and the execCtx /
//	              queryCtx funnels every statement goes through
//	cache.go      entityMeta, computed once per entity type by reflection
//	              (naming.go/english.go derive snake_case table and column
//	              names; english.go holds the pluralisation engine)
//	query.go      the fluent DSL: Select/WhereCol/IsNull/... plus All/ByIds,
//	              accumulating the first error instead of panicking
//	exec.go       write mode: Set/Execute/UpdateById, and the execUpdate /
//	              execDelete exits
//	model.go      free-function CRUD: Save/Update/Delete over any entity
//	mapper.go     row scanning: scanReflect maps rows via one typed
//	              destination per column, assignValue is the single type-
//	              conversion funnel
//	tx.go         transactions: Atomic, nested SAVEPOINTs, Unchecked as the
//	              one documented escape hatch for custom rollback rules
//	dialect/      SQL rendering. Base implements standard CRUD; each real
//	              dialect only overrides Paginate/Quote
//
// # Design contracts
//
// Three invariants hold everywhere and explain most of the code shape:
//
//   - SQLParams is pure data. The dialect layer never sees entities or
//     reflection; it renders strings from column names and placeholder args.
//   - Entity metadata is immutable after first use. metaOf caches per
//     reflect.Type; scanning paths never re-derive names.
//   - Every statement funnels through execCtx (write) or queryCtx (read) on
//     the Anima instance, so timing, error wrapping, and driver access have
//     exactly one implementation each.
//
// # Error model
//
// Operations return *Error carrying a code and an unwrappable cause, with
// the first error kept in query builders. Rollback decisions go through
// defaultRollbackWhen: only RuntimeExceptions (uncheckedErr) and runtime
// errors trigger them, which is why SaveBatch wraps element errors with
// Unchecked to promise whole-batch rollback.
package anima
