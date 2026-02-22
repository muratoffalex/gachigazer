package ask

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/muratoffalex/gachigazer/internal/ai"
	"github.com/muratoffalex/gachigazer/internal/config"
	"github.com/muratoffalex/gachigazer/internal/markdown"
	"github.com/muratoffalex/gachigazer/internal/service"
)

type Section int

const (
	SectionContent Section = iota
	SectionReasoning
	SectionContext
	SectionMetadata
	SectionPrompt
)

type BuilderConfig struct {
	ShowContext   bool
	ShowReasoning bool
	ShowMetadata  bool
	SectionsOrder []Section
	Separators    map[Section]string
}

type CommandArgs struct {
	Model        string
	Context      string
	SearchWeb    bool
	HandleImages bool
	HandleFiles  bool
	HandleAudio  bool
	HandleURLs   bool
	Recursive    bool
	Reasoning    *bool
	Tools        string
	Think        bool
	Multi        bool
	Fast         bool
	RF           bool
	Stream       *bool
	Temperature  *float32
	TopP         *float32
	Prompt       string
	ChainID      int
	New          bool
}

type MetadataUsage struct {
	Total  int64
	Input  int64
	Output int64
	Cost   float64
}

func NewMetadataUsageFrom(usage *ai.ModelUsage) *MetadataUsage {
	u := &MetadataUsage{}
	if usage == nil {
		return u
	}
	u.Cost = usage.GetCost()
	u.Input = usage.PromptTokens
	u.Output = usage.CompletionTokens
	u.Total = usage.TotalTokens
	return u
}

func (u *MetadataUsage) Add(usage *MetadataUsage) {
	u.Cost += usage.Cost
	u.Input += usage.Input
	u.Output += usage.Output
	u.Total += usage.Total
}

func (u *MetadataUsage) GetFormattedString(isTotal bool, currency *config.CurrencyConfig, l *service.Localizer) string {
	var totalCostStr string
	if u.Cost > 0 {
		totalCostDollars := u.Cost
		precision := countSignificantDecimals(totalCostDollars)
		if currency != nil && currency.Precision > 0 {
			precision = currency.Precision
		}
		totalCostStr = fmt.Sprintf(" $%.*f", precision, totalCostDollars)

		if currency != nil && currency.Code != "" {
			if rate, err := service.GetCurrencyService().GetUSDRate(context.Background(), currency.Code); err == nil {
				totalCostInCurrency := totalCostDollars * rate
				if currency.Symbol == "" {
					currency.Symbol = currency.Code
				}
				precision = currency.Precision
				if precision == 0 {
					precision = countSignificantDecimals(totalCostInCurrency)
				}
				totalCostStr = fmt.Sprintf(" ≈%s%.*f", currency.Symbol, precision, totalCostInCurrency)
			} else {
				fmt.Printf("Currency error: %v\n", err)
			}
		}
	}
	label := l.Localize("ask.response.tokens", nil)
	if isTotal {
		label = l.Localize("ask.response.totalTokens", nil)
	}
	return fmt.Sprintf(
		"*%s:* %s \\(%s\\+%s\\)%s",
		label,
		markdown.Escape(fmt.Sprint(u.Total)),
		markdown.Escape(fmt.Sprint(u.Input)),
		markdown.Escape(fmt.Sprint(u.Output)),
		markdown.Escape(totalCostStr),
	)
}

type Metadata struct {
	Model           *ai.ModelInfo
	Provider        ai.Provider
	Usage           *MetadataUsage
	MessageID       int
	TotalUsage      *MetadataUsage
	ContextTurns    int
	ContinueChainID int
	ChatID          int64
	ConversationID  int64
	ModelParams     *ai.ModelParams
	Currency        *config.CurrencyConfig
	l               *service.Localizer
}

func NewMetadata(model *ai.ModelInfo, provider ai.Provider, tokens *MetadataUsage, messageID int, totalTokens *MetadataUsage, contextTurns int, continueChainID int, chatID int64, conversationID int64, params *ai.ModelParams, currency *config.CurrencyConfig, localizer *service.Localizer) Metadata {
	return Metadata{
		Model:           model,
		Provider:        provider,
		Usage:           tokens,
		MessageID:       messageID,
		TotalUsage:      totalTokens,
		ContextTurns:    contextTurns,
		ContinueChainID: continueChainID,
		ChatID:          chatID,
		ConversationID:  conversationID,
		ModelParams:     params,
		Currency:        currency,
		l:               localizer,
	}
}

func (m *Metadata) GetDetailedInfo() string {
	var params []string
	if m.ModelParams.Stream != nil {
		params = append(params, fmt.Sprintf("*Stream:* %t", *m.ModelParams.Stream))
	}
	if m.ModelParams.Temperature != nil {
		params = append(params, fmt.Sprintf("*Temperature:* %s", markdown.Escape(fmt.Sprintf("%.1f", *m.ModelParams.Temperature))))
	}
	if m.ModelParams.MaxTokens != nil {
		params = append(params, fmt.Sprintf("*Max Tokens:* %d", *m.ModelParams.MaxTokens))
	}
	if m.ModelParams.TopP != nil {
		params = append(params, fmt.Sprintf("*Top P:* %s", markdown.Escape(fmt.Sprintf("%.1f", *m.ModelParams.TopP))))
	}
	if m.ModelParams.Reasoning != nil {
		reasoningParams := []string{}

		if m.ModelParams.Reasoning.Enabled != nil {
			reasoningParams = append(reasoningParams, fmt.Sprintf("  • Enabled: %t", *m.ModelParams.Reasoning.Enabled))
		}
		if m.ModelParams.Reasoning.Exclude != nil {
			reasoningParams = append(reasoningParams, fmt.Sprintf("  • Exclude: %t", *m.ModelParams.Reasoning.Exclude))
		}
		if m.ModelParams.Reasoning.MaxTokens != nil {
			reasoningParams = append(reasoningParams, fmt.Sprintf("  • Max Tokens: %d", *m.ModelParams.Reasoning.MaxTokens))
		}
		if m.ModelParams.Reasoning.Effort != nil {
			reasoningParams = append(reasoningParams, fmt.Sprintf("  • Effort: %s", *m.ModelParams.Reasoning.Effort))
		}

		if len(reasoningParams) > 0 {
			params = append(params, "*Reasoning:*")
			params = append(params, reasoningParams...)
		}
	}

	return fmt.Sprintf(
		m.l.Localize("ask.response.parameters", nil)+"\n%s",
		strings.Join(params, "\n"),
	)
}

func (m *Metadata) GetFormattedString() string {
	formatted := []string{}
	if m.Model != nil {
		modalities := m.Model.GetFormattedInputModalities()
		if modalities == "" {
			modalities = "❓"
		}
		toolsIcon := ""
		if m.Model.SupportsTools() {
			toolsIcon = ai.Tools
		}
		formatted = append(formatted, fmt.Sprintf(
			"*%s:* %s%s `%s`",
			m.l.Localize("ask.response.model", nil),
			modalities,
			toolsIcon,
			markdown.Escape(m.Model.FullName()),
		))
	}
	if m.Usage != nil && m.Usage.Total > 0 {
		formatted = append(formatted, m.Usage.GetFormattedString(false, m.Currency, m.l))
	}
	if m.TotalUsage != nil && m.Usage != nil && m.TotalUsage.Total > m.Usage.Total {
		formatted = append(formatted, m.TotalUsage.GetFormattedString(true, m.Currency, m.l))
	}
	if m.ContextTurns != 0 {
		item := fmt.Sprintf(
			"*%s:* %d",
			m.l.Localize("ask.response.contextTurns", nil),
			m.ContextTurns,
		)
		if m.ContinueChainID != 0 {
			item += fmt.Sprintf(
				" · [%s](https://t.me/c/%d/%d)",
				m.l.Localize("ask.response.previousMessage", nil),
				m.ChatID,
				m.ContinueChainID,
			)
		}
		formatted = append(formatted, item)
	}
	if m.MessageID != 0 && m.ConversationID != 0 {
		formatted = append(formatted, fmt.Sprintf("`/a $id:%d` · \\#ch%d", m.MessageID, m.ConversationID))
	}
	return strings.Join(formatted, "\n")
}

type ContextToolDetailed struct {
	Name     string
	Params   map[string]any
	Response string
}

type AudioInput struct {
	Name string
	Type string
}

type Context struct {
	Images                 []string
	Tools                  []string
	Files                  []string
	Audio                  []AudioInput
	URLs                   []*URLInfo
	DetailedTools          []ContextToolDetailed
	Additional             []string
	SeparatedModelForTools bool
}

func NewContext() Context {
	return Context{
		Images:                 make([]string, 0),
		Tools:                  make([]string, 0),
		Files:                  make([]string, 0),
		URLs:                   make([]*URLInfo, 0),
		DetailedTools:          make([]ContextToolDetailed, 0),
		Additional:             make([]string, 0),
		SeparatedModelForTools: false,
	}
}

func (c *Context) SetSeparatedModelForTools(value bool) {
	c.SeparatedModelForTools = value
}

func (c *Context) AddTool(name string) {
	c.Tools = append(c.Tools, name)
}

func (c *Context) AddDetailedTool(tool ContextToolDetailed) {
	c.DetailedTools = append(c.DetailedTools, tool)
}

func (c *Context) AddImageURL(url string) {
	c.Images = append(c.Images, url)
}

func (c *Context) AddFile(url string) {
	c.Files = append(c.Files, url)
}

func (c *Context) AddAudio(format string) {
	c.Audio = append(c.Audio, AudioInput{Type: format})
}

func (c *Context) AddURL(url *URLInfo) {
	c.URLs = append(c.URLs, url)
}

func (c *Context) AddAdditional(additional string) {
	c.Additional = append(c.Additional, additional)
}

func (c *Context) GetFormattedString(
	model *ai.ModelInfo,
	provider ai.Provider,
	l *service.Localizer,
	detailed bool,
) string {
	formatted := []string{}
	if len(c.Images) > 0 {
		item := fmt.Sprintf(
			"*%s:* %d",
			l.Localize("ask.response.images", nil),
			len(c.Images),
		)
		// Check if all images are preprocessed
		allPreprocessed := true
		for _, img := range c.Images {
			if img != "preprocessed" {
				allPreprocessed = false
				break
			}
		}
		if !allPreprocessed && !model.SupportsImageRecognition() {
			item += " · ⚠️ " + l.Localize("ask.response.modelDoesntSupportImages", nil)
		}
		formatted = append(formatted, item)
	}
	if len(c.Audio) > 0 {
		item := fmt.Sprintf(
			"*%s:* %d",
			l.Localize("ask.response.audioInputs", nil),
			len(c.Audio),
		)
		if !model.SupportsAudioRecognition() {
			item += " · ⚠️ " + l.Localize("ask.response.modelDoesntSupportAudio", nil)
		}
		formatted = append(formatted, item)
	}
	if len(c.Files) > 0 {
		item := fmt.Sprintf("*%s*", l.Localize("ask.response.files", nil))
		_, isOpenrouter := provider.(*ai.OpenRouterClient)
		if !model.SupportsFiles() && !isOpenrouter {
			item += " · ⚠️ " + l.Localize("ask.response.modelDoesntSupportPDF", nil)
		}
		item += "\n"
		for _, file := range c.Files {
			item += markdown.Escape(file) + "\n"
		}
		item = strings.TrimSpace(item)
		formatted = append(formatted, item)
	}
	if len(c.URLs) > 0 {
		item := fmt.Sprintf("*%s:*\n", l.Localize("ask.response.urls", nil))
		for _, url := range c.URLs {
			item += url.FormattedString(detailed, l) + "\n"
		}
		formatted = append(formatted, strings.TrimSpace(item))
	}
	if len(c.Tools) > 0 {
		item := fmt.Sprintf(
			"*%s:* %s",
			l.Localize("ask.response.tools", nil),
			markdown.Escape(strings.Join(c.Tools, ", ")),
		)
		if !model.SupportsTools() && !c.SeparatedModelForTools {
			item += " · ⚠️ " + l.Localize("ask.response.modelDoesntSupportTools", nil)
		}
		formatted = append(formatted, strings.TrimSpace(item))
	}
	if len(c.DetailedTools) > 0 {
		title := fmt.Sprintf("*%s*", l.Localize("ask.response.usedTools", nil))
		if !model.SupportsTools() {
			title += " · ⚠️ " + l.Localize("ask.response.modelDoesntSupportTools", nil)
		}
		toolsSlice := []string{title}

		for _, item := range c.DetailedTools {
			marshaledParams, err := json.Marshal(item.Params)
			response := strings.TrimSpace(markdown.Escape(item.Response))
			if len(response) > 1000 {
				response = response[:1000] + markdown.Escape(
					fmt.Sprintf(
						"... [%s]",
						l.Localize("ask.response.truncated", nil),
					),
				)
			}
			response = strings.TrimSpace(response)
			response = "> " + strings.ReplaceAll(response, "\n", "\n> ") + "||"
			if err == nil {
				toolsSlice = append(toolsSlice, fmt.Sprintf(
					"*%s*: %s\n%s",
					markdown.Escape(item.Name),
					markdown.Escape(string(marshaledParams)),
					response,
				))
			}
		}

		toolsString := strings.Join(toolsSlice, "\n")

		formatted = append(formatted, strings.TrimSpace(toolsString))
	}
	if len(c.Additional) > 0 {
		item := fmt.Sprintf("*%s:*\n", l.Localize("ask.response.additionalContext", nil))
		for _, additional := range c.Additional {
			item += strings.TrimSpace(additional) + "\n"
		}
		formatted = append(formatted, strings.TrimSpace(item))
	}
	return strings.Join(formatted, "\n")
}

type Response struct {
	Prompt    string
	Reasoning string
	Content   string
	Context   Context
	Metadata  Metadata
}

func NewResponse() *Response {
	return &Response{
		Prompt:    "",
		Reasoning: "",
		Content:   "",
		Context:   Context{},
		Metadata:  Metadata{},
	}
}

func (r *Response) SetReasoning(text string) {
	r.Reasoning = strings.TrimSpace(text)
}

func (r *Response) SetPrompt(text string) {
	r.Prompt = strings.TrimSpace(text)
}

func (r *Response) SetContent(text string) {
	r.Content = strings.TrimSpace(text)
}

func (r *Response) HasContent() bool {
	return len(r.Content) > 0
}

func (r *Response) HasReasoning() bool {
	return len(r.Reasoning) > 0
}
