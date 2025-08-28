package ask

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode"
)

func countSignificantDecimals(f float64) int {
	str := fmt.Sprintf("%f", f)
	parts := strings.Split(str, ".")
	if len(parts) < 2 {
		return 0
	}
	decimalPart := parts[1]
	for i, ch := range decimalPart {
		if ch != '0' {
			return len(decimalPart) - i
		}
	}
	return 0
}

func formatForwardOrigin(fo *forwardOrigin) string {
	replyHeader := fmt.Sprintf(" [forwarded from %s %s", fo.Type, fo.Name)
	if fo.EncodedID != "" {
		replyHeader += fmt.Sprintf("(%s)", fo.EncodedID)
	}
	if fo.Username != "" && fo.Type == "channel" {
		replyHeader += fmt.Sprintf(
			" | Channel name: %s | Post ID: %d",
			fo.Username,
			fo.MessageID,
		)
	}
	return replyHeader + "]"
}

func extractAndRemovePattern(text string) (string, string) {
	re := regexp.MustCompile(`!\S+`)
	match := re.FindString(text)
	if match != "" {
		match = strings.ReplaceAll(match, "-", " ")
		return strings.TrimSpace(strings.Replace(text, match, "", 1)), match
	}
	return text, ""
}

func capitalizeFirst(str string) string {
	if len(str) == 0 {
		return str
	}
	r := []rune(str)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

func extractStringSlice(v reflect.Value) []string {
	result := make([]string, v.Len())
	for i := range v.Len() {
		result[i] = v.Index(i).String()
	}
	return result
}
