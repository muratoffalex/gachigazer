package ask

import (
	"fmt"
	"strings"

	"github.com/muratoffalex/gachigazer/internal/markdown"
	"github.com/muratoffalex/gachigazer/internal/service"
	"github.com/muratoffalex/gachigazer/internal/telegram"
)

const (
	telegramMaxLength         = 4096
	promptTitle               = ">*%s:* "
	promptTitleTelegramify    = "**>_%s:_ "
	reasoningTitle            = promptTitle
	reasoningTitleTelegramify = promptTitleTelegramify
)

type MessageBuilder struct {
	tg       telegram.Client
	response *Response
	config   BuilderConfig
	l        *service.Localizer
}

func NewMessageBuilder(tg telegram.Client, localizer *service.Localizer) *MessageBuilder {
	return &MessageBuilder{
		tg:       tg,
		response: NewResponse(),
		config: BuilderConfig{
			ShowContext:   true,
			ShowReasoning: true,
			ShowMetadata:  true,
			SectionsOrder: []Section{SectionPrompt, SectionReasoning, SectionContent, SectionContext, SectionMetadata},
			Separators: map[Section]string{
				SectionContent: "──────",
				SectionContext: " ",
			},
		},
		l: localizer,
	}
}

func (b *MessageBuilder) getPromptTitle() (string, string) {
	return fmt.Sprintf(promptTitle, b.l.Localize("ask.response.promptTitle", nil)), fmt.Sprintf(promptTitleTelegramify, b.l.Localize("ask.response.promptTitle", nil))
}

func (b *MessageBuilder) getReasoningTitle() (string, string) {
	return fmt.Sprintf(reasoningTitle, b.l.Localize("ask.response.reasoningTitle", nil)), fmt.Sprintf(reasoningTitleTelegramify, b.l.Localize("ask.response.reasoningTitle", nil))
}

func (b *MessageBuilder) SetResponse(resp *Response) *MessageBuilder {
	b.response = resp
	return b
}

func (b *MessageBuilder) WithSectionOrder(order ...Section) *MessageBuilder {
	b.config.SectionsOrder = order
	return b
}

func (b *MessageBuilder) WithSeparator(after Section, sep string) *MessageBuilder {
	b.config.Separators[after] = sep
	return b
}

func (b *MessageBuilder) WithContext(show bool) *MessageBuilder {
	b.config.ShowContext = show
	return b
}

func (b *MessageBuilder) WithMetadata(show bool) *MessageBuilder {
	b.config.ShowMetadata = show
	return b
}

func (b *MessageBuilder) WithReasoning(show bool) *MessageBuilder {
	b.config.ShowReasoning = show
	return b
}

func (b *MessageBuilder) SetSeparator(sep string) *MessageBuilder {
	b.config.Separators[SectionContent] = sep
	return b
}

func (b *MessageBuilder) buildContent() (string, error) {
	if b.response.Content == "" {
		return "", nil
	}

	content, err := b.tg.TelegramifyMarkdown(b.response.Content)
	if err != nil {
		content = b.tg.EscapeText(b.response.Content)
	}
	return strings.TrimSpace(content), nil
}

func (b *MessageBuilder) buildPrompt() (string, error) {
	if b.response.Prompt == "" {
		return "", nil
	}

	title, _ := b.getPromptTitle()
	prompt := title + strings.ReplaceAll(b.response.Prompt, "\n", " ")
	escaped, err := b.tg.TelegramifyMarkdown(prompt)
	if err != nil {
		escaped = b.tg.EscapeText(prompt)
	}
	return strings.TrimSpace(escaped), nil
}

func (b *MessageBuilder) buildReasoning() (string, error) {
	if !b.config.ShowReasoning || b.response.Reasoning == "" {
		return "", nil
	}

	title, _ := b.getReasoningTitle()
	reasoning := title + b.response.Reasoning
	escaped, err := b.tg.TelegramifyMarkdown(reasoning)
	if err != nil {
		escaped = b.tg.EscapeText(reasoning)
	}
	return strings.TrimSpace(escaped), nil
}

func (b *MessageBuilder) buildContext() (string, error) {
	if !b.config.ShowContext {
		return "", nil
	}
	return strings.TrimSpace(b.response.Context.GetFormattedString(b.response.Metadata.Model, b.response.Metadata.Provider, b.l, false)), nil
}

func (b *MessageBuilder) buildMetadata() (string, error) {
	if !b.config.ShowMetadata {
		return "", nil
	}
	return strings.TrimSpace(b.response.Metadata.GetFormattedString()), nil
}

func (b *MessageBuilder) Reasoning() bool {
	return b.config.ShowReasoning && b.response.Reasoning != ""
}

func (b *MessageBuilder) buildAllSections() map[Section]string {
	sections := make(map[Section]string)

	if content, _ := b.buildPrompt(); content != "" {
		sections[SectionPrompt] = content
	}
	if content, _ := b.buildContent(); content != "" {
		sections[SectionContent] = content
	}
	if reasoning, _ := b.buildReasoning(); reasoning != "" {
		sections[SectionReasoning] = reasoning
	}
	if context, _ := b.buildContext(); context != "" {
		sections[SectionContext] = context
	}
	if metadata, _ := b.buildMetadata(); metadata != "" {
		sections[SectionMetadata] = metadata
	}

	return sections
}

func (b *MessageBuilder) shouldAddSeparator(section Section, index int) bool {
	_, hasSeparator := b.config.Separators[section]
	return hasSeparator && index < len(b.config.SectionsOrder)-1
}

func (b *MessageBuilder) Build() string {
	builtSections := b.buildAllSections()
	final := b.buildWithSections(builtSections)

	if len(final) <= telegramMaxLength {
		return final
	}

	maxLengthReachedEscaped := markdown.Escape(
		"... " + b.l.Localize("ask.maxLengthReached", nil),
	)
	// TODO: just subtract the message length (n1) from the total max length,
	// if the message doesn't fit, then trim the reasoning by n1

	// Try to trim reasoning first
	if reasoning, exists := builtSections[SectionReasoning]; exists {
		_, titleTelegramify := b.getReasoningTitle()
		// Calculate available space for reasoning
		otherParts := make([]string, 0, len(builtSections)-1)
		for sec, content := range builtSections {
			if sec != SectionReasoning {
				otherParts = append(otherParts, content)
			}
		}
		otherContent := strings.Join(otherParts, "\n")
		otherContent = cleanText(otherContent) + BotMessageMarker

		availableSpace := telegramMaxLength - len(otherContent) - len(markdown.Escape(b.config.Separators[SectionContent])) - len(markdown.Escape(b.config.Separators[SectionContext])) - 2
		if availableSpace > len(titleTelegramify+maxLengthReachedEscaped+"||") {
			// Trim reasoning to fit available space
			maxReasoningLength := availableSpace - len(titleTelegramify) - len(maxLengthReachedEscaped) - len("||")
			reasoningWithoutTitle := strings.TrimSpace(reasoning[len(titleTelegramify):])
			trimmedReasoning := reasoning
			if len(trimmedReasoning) > maxReasoningLength {
				trimmedReasoning = reasoningWithoutTitle[:maxReasoningLength]
				trimmedReasoning = titleTelegramify + trimmedReasoning + maxLengthReachedEscaped + "||"
			}
			builtSections[SectionReasoning] = trimmedReasoning
			final = b.buildWithSections(builtSections)
		} else {
			delete(builtSections, SectionReasoning)
			final = b.buildWithSections(builtSections)
		}
	}

	if len(final) <= telegramMaxLength {
		return final
	}

	if builtSections[SectionPrompt] != "" {
		builtSections[SectionPrompt] = ""
		final = b.buildWithSections(builtSections)
	}

	// Last resort - trim content
	if content, exists := builtSections[SectionContent]; exists {
		remainingLength := telegramMaxLength - len(final)
		if remainingLength < 0 {
			truncateAt := len(content) + remainingLength
			telegramified, _ := b.tg.TelegramifyMarkdown(content[:truncateAt])
			builtSections[SectionContent] = telegramified + maxLengthReachedEscaped
		}
		final = b.buildWithSections(builtSections)
	}

	return final
}

func (b *MessageBuilder) buildWithSections(sections map[Section]string) string {
	var parts []string
	for i, section := range b.config.SectionsOrder {
		if content, exists := sections[section]; exists {
			parts = append(parts, content)

			if b.shouldAddSeparator(section, i) && (sections[SectionMetadata] != "" || sections[SectionContext] != "") {
				parts = append(parts, markdown.Escape(b.config.Separators[section]))
			}
		}
	}

	final := strings.Join(parts, "\n")
	return cleanText(final) + BotMessageMarker
}
