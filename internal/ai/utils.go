package ai

import "strings"

func ParseModelSpec(modelSpec string) (provider string, model string, err error) {
	parts := strings.SplitN(modelSpec, ":", 2)
	if len(parts) != 2 {
		return "", modelSpec, ErrInvalidModelFormat
	}
	return parts[0], parts[1], nil
}

func HandleContentReasoning(text string) (content, reasoning string) {
	content = text
	switch {
	case strings.Contains(text, "Reasoning:"):
		parts := strings.SplitN(text, "Reasoning:", 2)
		content = strings.TrimSpace(parts[0])
		reasoning = strings.TrimSpace(parts[1])

	case strings.Contains(text, "<reasoning>"):
		start := strings.Index(text, "<reasoning>")
		end := strings.Index(text, "</reasoning>")
		if start >= 0 && end > start {
			reasoning = strings.TrimSpace(content[start+11 : end])
			content = strings.TrimSpace(content[:start] + content[end+12:])
		}

	case strings.Contains(content, "```reasoning"):
		start := strings.Index(content, "```reasoning")
		end := strings.Index(content, "```")
		if start >= 0 && end > start && end != start {
			reasoning = strings.TrimSpace(content[start+12 : end])
			content = strings.TrimSpace(content[:start] + content[end+3:])
		}
	}
	return
}
