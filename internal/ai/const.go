package ai

const (
	Free  = "🆓"
	Tools = "🛠️"

	ImageGenerationModality  = "🖼️"
	ImageRecognitionModality = "👁️"
	TextModality             = "💬"
	FileModality             = "📄"
	AudioModality            = "🎵"

	ProviderOpenrouter = "openrouter"
	ProviderOpenai     = "openai-compatible"
	ProviderLocal      = "local"

	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
	RoleTool      = "tool"

	RoleInternal = "internal"
)

var SupportedRoles = []string{
	RoleSystem,
	RoleUser,
	RoleAssistant,
	RoleTool,
}
