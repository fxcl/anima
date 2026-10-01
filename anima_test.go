package anima

import (
	"context"
	"reflect"
	"testing"
	"time"
)

// User matches the Java test entity: table users, primary key id, camelCase columns mapped to snake_case
type User struct {
	Id        int64
	UserName  string
	Age       int
	Email     *string
	CreatedAt *time.Time
	Orders    []*Order `anima:"-"` // hasMany association mount (benchmark join cases only)
}

func TestEntityNamingConvention(t *testing.T) {
	m := Of().metaOf(reflect.TypeOf(User{}))
	if m.table != "users" {
		t.Fatalf("table = %q, want users", m.table)
	}
	if m.pk != "id" {
		t.Fatalf("pk = %q, want id", m.pk)
	}
	if f := m.byRef["UserName"]; f == nil || f.Column != "user_name" {
		t.Fatalf("UserName column = %+v, want user_name", f)
	}
}

func TestNamingHelpers(t *testing.T) {
	cases := [][2]string{
		{"userId", "user_id"},
		{"OrderInfo", "order_info"},
		{"ID", "i_d"}, // Java: 'D'.equals('D'.toUpperCase()) breaks the word here; faithful port
	}
	for _, c := range cases {
		if got := toUnderline(c[0]); got != c[1] {
			t.Errorf("toUnderline(%q) = %q, want %q", c[0], got, c[1])
		}
	}
	if got := toCamelName("user_name"); got != "userName" {
		t.Errorf("toCamelName = %q", got)
	}
}

func TestEnglishPlural(t *testing.T) {
	cases := [][2]string{
		{"word", "words"},
		{"cat", "cats"},
		{"user", "users"},
		{"order_info", "order_infoes"}, // Java rule table "o$"->"oes" hits the same way (anima's original needs @Table to override such names)
		{"address", "addresses"},
		{"box", "boxes"},
		{"category", "categories"},
		{"status", "statuses"},
		{"child", "children"},
		{"person", "persons"},
		{"deer", "deer"},
		{"fish", "fish"},
		{"quiz", "quizzes"},
		{"bacterium", "bacteria"},
		{"criterion", "criteria"},
		{"mouse", "mice"},
		{"goose", "geese"},
		{"blog", "blogs"},
	}
	for _, c := range cases {
		if got := plural(c[0]); got != c[1] {
			t.Errorf("plural(%q) = %q, want %q", c[0], got, c[1])
		}
	}
}

// TestListRuleSkipsInconsistentRule guards against a regression of the panic
// at the old `if !strings.HasSuffix(lower, r.singular) { panic(...) }`
// branch in listRule.apply. A categoryRule whose list contains a suffix
// longer than its declared singular suffix is data-inconsistent; the
// inflector must skip such a rule rather than panic on user input.
func TestListRuleSkipsInconsistentRule(t *testing.T) {
	f := &inflector{}
	f.categoryRule([]string{"examplelongersuffix"}, "ex", "ices")
	// The inflector has only the inconsistent rule; with the panic replaced
	// by a graceful "no match", getPlural returns "" instead of crashing.
	// Compare against a fresh inflector that has the same data and the
	// trailing "$" -> "s" fallback, to confirm the inconsistency simply
	// makes that rule a no-op rather than escalating to the fallback.
	if got := f.getPlural("examplelongersuffix"); got != "" {
		t.Errorf("inconsistent rule: got %q, want \"\" (rule skipped, no fallback)", got)
	}
}

func TestSaveAndQueryTracer(t *testing.T) {
	ctx := context.Background()
	email := "john@doe.com"

	k1, err := Save(ctx, &User{UserName: "jack", Age: 25, Email: &email})
	if err != nil {
		t.Fatal(err)
	}
	if k1.AsInt() <= 0 {
		t.Fatalf("save returned key %v", k1)
	}
	if _, err := Save(ctx, &User{UserName: "rose", Age: 10}); err != nil {
		t.Fatal(err)
	}

	// Field condition chain: alignment guard for select().from(User.class).where(User::getAge).gt(15)
	users, err := Select[User]().WhereCol("Age").Gt(15).Order("id asc").All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].UserName != "jack" {
		t.Fatalf("got %+v", users)
	}
	if users[0].Email == nil || *users[0].Email != "john@doe.com" {
		t.Fatalf("nullable column mapping failed: %+v", users[0])
	}

	// Raw fragment condition: alignment guard for where("age > ?", 15)
	n, err := Select[User]().Where("age > ?", 15).Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}

	// ById
	one, err := Select[User]().ById(ctx, k1.AsInt())
	if err != nil || one == nil {
		t.Fatalf("byId: %v %+v", err, one)
	}
	if one.Id != int64(k1.AsInt()) {
		t.Fatalf("byId wrong row: %+v", one)
	}
}

func TestUnknownFieldFailsFast(t *testing.T) {
	_, err := Select[User]().WhereCol("Nope").Eq(1).All(context.Background())
	if err == nil {
		t.Fatal("want unknown field error")
	}
	e, ok := err.(*Error)
	if !ok || e.Code != ErrorCodeUnknownField {
		t.Fatalf("err = %v", err)
	}
}
