package anima

import (
	"strings"
	"unicode"
)

// toUnderline converts camelCase to snake_case: userId -> user_id.
// Any character without a lowercase form (uppercase letters, digits,
// punctuation) triggers a word break.
// ToUnderline converts camelCase to snake_case (Java AnimaUtils.toUnderline):
// userId -> user_id.
func ToUnderline(value string) string { return toUnderline(value) }

// ToCamelName converts snake_case to lower camelCase (Java
// AnimaUtils.toCamelName): user_id -> userId.
func ToCamelName(value string) string { return toCamelName(value) }

func toUnderline(value string) string {
	if value == "" {
		return value
	}
	var sb strings.Builder
	runes := []rune(value)
	sb.WriteRune(unicode.ToLower(runes[0]))
	for _, r := range runes[1:] {
		if unicode.IsLower(r) {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('_')
			sb.WriteRune(unicode.ToLower(r))
		}
	}
	return sb.String()
}

// toCamelName converts snake_case to camelCase: user_id -> userId
func toCamelName(value string) string {
	parts := strings.Split(value, "_")
	var sb strings.Builder
	sb.WriteString(parts[0])
	for _, p := range parts[1:] {
		if p == "" {
			continue
		}
		sb.WriteString(strings.ToUpper(p[:1]))
		sb.WriteString(p[1:])
	}
	return sb.String()
}
