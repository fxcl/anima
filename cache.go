package anima

import (
	"database/sql"
	"reflect"
	"strings"
	"sync"
)

// fieldMeta holds the metadata of a single mapped field.
type fieldMeta struct {
	Name   string         // Go field name, e.g. UserName
	Column string         // column name, e.g. user_name
	Index  int            // struct field index
	IsPK   bool
	Type   reflect.Type   // field type (the declared type, before pointer-deref)
	// Pre-computed scan strategy hints, populated once in computeMeta. They
	// determine whether dst.Addr().Interface() boxing into the corresponding
	// interface is worth attempting. Computed against the leaf type after
	// stripping pointers, matching the recursion in convertAssign.
	hasDBEnumScanner bool
	hasSQLScanner    bool
}

var (
	dbEnumScannerType = reflect.TypeOf((*DBEnumScanner)(nil)).Elem()
	sqlScannerType    = reflect.TypeOf((*sql.Scanner)(nil)).Elem()
)

// entityMeta holds the entity type metadata (centralized cache).
type entityMeta struct {
	typ      reflect.Type
	table    string
	pk       string // primary key column; empty when the entity has none
	pkIndex  int
	fields   []fieldMeta
	byRef    map[string]*fieldMeta // Go field name or column -> field metadata
	byColumn map[string]*fieldMeta
	ignored  map[string]int // anima:"-" field name -> struct index (join mount points)
}

// TableNameProvider is an optional interface an entity can implement to
// override its table name.
type TableNameProvider interface {
	TableName() string
}

type metaKey struct {
	t      reflect.Type
	prefix string
}

var metaCache sync.Map // metaKey -> *entityMeta

// metaOf returns (and caches) the entity type metadata. t must be a struct
// type.
func (a *Anima) metaOf(t reflect.Type) *entityMeta {
	key := metaKey{t: t, prefix: a.tablePrefix}
	if cached, ok := metaCache.Load(key); ok {
		return cached.(*entityMeta)
	}
	m := computeMeta(t, a.tablePrefix)
	actual, _ := metaCache.LoadOrStore(key, m)
	return actual.(*entityMeta)
}

// requirePK guards against missing primary keys.
func (m *entityMeta) requirePK() error {
	if m.pk == "" {
		return newError(ErrorCodeNoPrimaryKey, "entity %s has no primary key", m.typ.Name())
	}
	return nil
}

// fieldOf resolves field metadata by Go field name or column name.
// Unknown fields are recorded at build time and surface at execution.
func fieldOf(m *entityMeta, name string) (*fieldMeta, error) {
	f, ok := m.byRef[name]
	if !ok {
		return nil, newError(ErrorCodeUnknownField, "unknown field %q on %s", name, m.typ.Name())
	}
	return f, nil
}

func computeMeta(t reflect.Type, prefix string) *entityMeta {
	m := &entityMeta{
		typ:      t,
		pkIndex:  -1,
		byRef:    map[string]*fieldMeta{},
		byColumn: map[string]*fieldMeta{},
		ignored:  map[string]int{},
	}

	if v, ok := reflect.New(t).Interface().(TableNameProvider); ok {
		m.table = v.TableName()
	} else {
		// plural(prefix + "_" + toUnderline(className))
		name := toUnderline(t.Name())
		if prefix != "" {
			name = prefix + "_" + name
		}
		m.table = plural(name)
	}

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.Anonymous || sf.PkgPath != "" {
			continue
		}
		switch sf.Type.Kind() {
		case reflect.Chan, reflect.Func:
			continue
		}
		tag := sf.Tag.Get("anima")
		if tag == "-" {
			m.ignored[sf.Name] = i
			continue
		}
		col, isPK := parseTag(tag)
		if col == "" {
			col = toUnderline(sf.Name)
		}
		m.fields = append(m.fields, fieldMeta{Name: sf.Name, Column: col, Index: i, IsPK: isPK, Type: sf.Type})
	}

	for i := range m.fields {
		f := &m.fields[i]
		// Scan hints are computed against the leaf type (after stripping
		// pointers). convertAssign recurses through pointer fields with
		// dst.Elem() and Addr() boxes the leaf, so this matches the runtime
		// receiver.
		leaf := f.Type
		for leaf.Kind() == reflect.Ptr {
			leaf = leaf.Elem()
		}
		if leaf.Kind() != reflect.Interface {
			ptrLeaf := reflect.PtrTo(leaf)
			f.hasDBEnumScanner = ptrLeaf.Implements(dbEnumScannerType)
			f.hasSQLScanner = ptrLeaf.Implements(sqlScannerType)
		}
	}

	for i := range m.fields {
		m.byRef[m.fields[i].Name] = &m.fields[i]
		m.byColumn[m.fields[i].Column] = &m.fields[i]
		// byRef also accepts column references (Go field names take priority
		// and are never overwritten by a column name)
		if _, exists := m.byRef[m.fields[i].Column]; !exists {
			m.byRef[m.fields[i].Column] = &m.fields[i]
		}
	}

	// Primary key: an explicit pk tag wins; otherwise the default column "id".
	for i := range m.fields {
		if m.fields[i].IsPK {
			m.pk, m.pkIndex = m.fields[i].Column, i
			break
		}
	}
	if m.pk == "" {
		for i := range m.fields {
			if m.fields[i].Column == "id" {
				m.fields[i].IsPK = true
				m.pk, m.pkIndex = "id", i
				break
			}
		}
	}
	return m
}

func parseTag(tag string) (col string, isPK bool) {
	for _, part := range strings.Split(tag, ",") {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "column:"):
			col = strings.TrimPrefix(part, "column:")
		case part == "pk":
			isPK = true
		}
	}
	return
}
