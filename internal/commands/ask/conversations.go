package ask

import (
	"database/sql"
	"slices"
	"time"

	"github.com/muratoffalex/gachigazer/internal/ai"
	"github.com/muratoffalex/gachigazer/internal/database"
)

const maxContextTurns = 100

type Role string

func (r Role) IsUser() bool {
	return r == ai.RoleUser
}

func (r Role) IsAssistant() bool {
	return r == ai.RoleAssistant
}

func (r Role) IsTool() bool {
	return r == ai.RoleTool
}

func (r Role) IsInternal() bool {
	return r == ai.RoleInternal
}

func (r Role) Supported() bool {
	return slices.Contains(ai.SupportedRoles, string(r))
}

type conversationMessage struct {
	ID                  int64
	ConversationChainID string
	ParentMessageID     sql.NullInt64
	ChatID              int64
	UserID              int64
	ToolName            sql.NullString
	ToolParams          map[string]any
	ToolCalls           []ai.ToolCall
	ToolResponses       []ai.Message
	MessageID           int
	ReplyToMessageID    sql.NullInt64
	Role                Role
	Text                string
	ModelName           sql.NullString
	Usage               *MetadataUsage
	AttemptsCount       uint8
	Params              *ai.ModelParams
	Annotations         []ai.AnnotationContent
	Images              []ai.Content
	Audio               []ai.Content
	Files               []ai.Content
	URLs                []*URLInfo
	ConversationID      int64
	ConversationTitle   sql.NullString
	ConversationSummary sql.NullString
	IsFirst             bool
	Saved               bool
	SavedAt             sql.NullTime
	SavedBy             *database.User
	CreatedAt           time.Time
}

func NewUserConversationMessage(
	parentMessageID int64,
	chatID int64,
	messageID int,
	replyToMessageID int64,
	conversationID int64,
	userID int64,
	conversationChainID string,
	text string,
	attempt uint8,
	images []ai.Content,
	files []ai.Content,
	audio []ai.Content,
	urls []*URLInfo,
	isFirst bool,
	tools []ai.Tool,
) *conversationMessage {
	return &conversationMessage{
		ParentMessageID:     sql.NullInt64{Int64: parentMessageID, Valid: true},
		ChatID:              chatID,
		MessageID:           messageID,
		ReplyToMessageID:    sql.NullInt64{Int64: replyToMessageID, Valid: true},
		ConversationID:      conversationID,
		UserID:              userID,
		ConversationChainID: conversationChainID,
		Role:                ai.RoleUser,
		Text:                text,
		AttemptsCount:       attempt,
		Images:              images,
		Files:               files,
		Audio:               audio,
		URLs:                urls,
		IsFirst:             isFirst,
	}
}

func NewAssistantConversationMessage(
	userMessage *conversationMessage,
	tgMessageID int,
	userID int64,
	text string,
	modelName string,
	params *ai.ModelParams,
	usageInfo *MetadataUsage,
	annotations []ai.AnnotationContent,
	toolCalls []ai.ToolCall,
) *conversationMessage {
	msg := &conversationMessage{
		ParentMessageID:     sql.NullInt64{Int64: userMessage.ID, Valid: true},
		ChatID:              userMessage.ChatID,
		MessageID:           tgMessageID,
		ReplyToMessageID:    sql.NullInt64{Int64: int64(userMessage.MessageID), Valid: true},
		ConversationID:      userMessage.ConversationID,
		UserID:              userID,
		ConversationChainID: userMessage.ConversationChainID,
		Role:                ai.RoleAssistant,
		Text:                text,
		ModelName:           sql.NullString{String: modelName, Valid: true},
		Params:              params,
		Usage:               usageInfo,
		Annotations:         annotations,
		ToolCalls:           toolCalls,
		IsFirst:             false,
	}
	return msg
}

func NewToolConversationMessage(
	assistantMessage *conversationMessage,
	text string,
	toolName string,
	toolParams map[string]any,
	toolResponses []ai.Message,
) *conversationMessage {
	msg := &conversationMessage{
		ParentMessageID:     sql.NullInt64{Int64: assistantMessage.ID, Valid: true},
		ReplyToMessageID:    assistantMessage.ReplyToMessageID,
		MessageID:           assistantMessage.MessageID,
		ChatID:              assistantMessage.ChatID,
		ConversationID:      assistantMessage.ConversationID,
		ConversationChainID: assistantMessage.ConversationChainID,
		Role:                ai.RoleTool,
		ToolName:            sql.NullString{String: toolName, Valid: true},
		Text:                text,
		ToolParams:          toolParams,
		ToolResponses:       toolResponses,
		IsFirst:             false,
	}
	return msg
}

func NewInternalConversationMessage(
	assistantMessage *conversationMessage,
	messageID int,
) *conversationMessage {
	msg := &conversationMessage{
		ParentMessageID:     sql.NullInt64{Int64: assistantMessage.ID, Valid: true},
		ReplyToMessageID:    sql.NullInt64{Int64: int64(assistantMessage.MessageID), Valid: true},
		MessageID:           messageID,
		ChatID:              assistantMessage.ChatID,
		ConversationID:      assistantMessage.ConversationID,
		ConversationChainID: assistantMessage.ConversationChainID,
		Role:                ai.RoleInternal,
		IsFirst:             false,
	}
	return msg
}
