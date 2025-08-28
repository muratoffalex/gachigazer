package ai

import "encoding/json"

// REQUEST

type ToolFunction struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Parameters  Parameters `json:"parameters"`
}

type Parameters struct {
	Type       string              `json:"type"`
	Properties map[string]Property `json:"properties"`
	Required   []string            `json:"required,omitzero"`
}

type Property struct {
	Type        string    `json:"type"`
	Enum        []string  `json:"enum,omitzero"`
	Description string    `json:"description,omitzero"`
	Items       *Property `json:"items,omitzero"`
}

type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

// RESPONSE

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

func (fc FunctionCall) GetArguments() (map[string]any, error) {
	var result map[string]any
	err := json.Unmarshal([]byte(fc.Arguments), &result)
	return result, err
}

type ToolCall struct {
	Index    int          `json:"index"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}
