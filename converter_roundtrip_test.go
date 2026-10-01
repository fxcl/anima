package anima

import (
	"fmt"
	"reflect"
	"testing"
)

// Status is a custom type with a registered Converter: it is stored as
// "live"/"draft" in the database instead of as an integer.
type Status int

const (
	StatusDraft Status = iota
	StatusLive
)

type statusConverter struct{}

func (statusConverter) Target() reflect.Type { return reflect.TypeOf(Status(0)) }

func (statusConverter) ToDatabase(v any) (any, error) {
	s, ok := v.(Status)
	if !ok {
		return nil, fmt.Errorf("statusConverter: unexpected type %T", v)
	}
	if s == StatusLive {
		return "live", nil
	}
	return "draft", nil
}

func (statusConverter) FromDatabase(v any) (any, error) {
	switch s := v.(type) {
	case string:
		if s == "live" {
			return StatusLive, nil
		}
		return StatusDraft, nil
	case []byte:
		if string(s) == "live" {
			return StatusLive, nil
		}
		return StatusDraft, nil
	default:
		return nil, fmt.Errorf("statusConverter: unexpected type %T", v)
	}
}

type Post struct {
	Id    int64
	Title string
	State Status
}

func TestConverterRoundTrip(t *testing.T) {
	ctx := t.Context()
	AddConverter(statusConverter{})

	if _, err := Execute(ctx, "DELETE FROM posts"); err != nil {
		t.Fatal(err)
	}
	if _, err := Save(ctx, &Post{Title: "hello", State: StatusLive}); err != nil {
		t.Fatal(err)
	}

	// The write path must store the converted string, not the integer.
	mu.RLock()
	db := defaultAnima.db
	mu.RUnlock()
	var raw string
	if err := db.QueryRowContext(ctx, "SELECT state FROM posts WHERE title = ?", "hello").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != "live" {
		t.Fatalf("ToDatabase not applied: state = %q, want %q", raw, "live")
	}

	// The read path must convert the column back into Status.
	got, err := Select[Post]().Where("title = ?", "hello").All(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].State != StatusLive {
		t.Fatalf("FromDatabase not applied: %+v", got)
	}
}
