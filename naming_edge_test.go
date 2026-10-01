package anima

import "testing"

// Edge pins for the naming helpers. Expectations probed from the actual
// implementation; both quirks are faithful Java-port behavior, not bugs:
//   - toUnderline breaks on ANY non-lowercase rune, so digits split words
//     ("userId2" -> "user_id_2") and an embedded underscore doubles
//     ("user_id" -> "user__id"); consecutive capitals each get their own
//     break ("HTTPServer" -> "h_t_t_p_server").
//   - toCamelName skips empty underscore-delimited parts, so leading and
//     trailing underscores change the result asymmetrically and digit-led
//     parts upper-case to themselves.
func TestNamingEdgeCases(t *testing.T) {
	underline := []struct{ in, want string }{
		{"", ""},
		{"a", "a"},
		{"A", "a"},
		{"userId2", "user_id_2"},
		{"HTTPServer", "h_t_t_p_server"},
		{"user_id", "user__id"},
		{"CamelCase", "camel_case"},
	}
	for _, c := range underline {
		if got := toUnderline(c.in); got != c.want {
			t.Errorf("toUnderline(%q) = %q, want %q", c.in, got, c.want)
		}
	}

	camel := []struct{ in, want string }{
		{"_id", "Id"},
		{"id_", "id"},
		{"user__name", "userName"},
		{"user_2fa", "user2fa"},
		{"userId", "userId"},
		{"a_b_c", "aBC"},
		{"_", ""},
	}
	for _, c := range camel {
		if got := toCamelName(c.in); got != c.want {
			t.Errorf("toCamelName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
