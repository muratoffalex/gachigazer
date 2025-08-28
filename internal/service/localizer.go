package service

import (
	"embed"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.toml
var localeFS embed.FS

type Localizer struct {
	bundle      *i18n.Bundle
	currentLang language.Tag
}

func NewLocalizer(currentLang string) (*Localizer, error) {
	localesDir := "locales"
	lang, err := language.Parse(currentLang)
	if err != nil {
		return nil, err
	}
	bundle := i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	files, err := localeFS.ReadDir(localesDir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if !strings.HasSuffix(file.Name(), ".toml") {
			continue
		}

		data, err := localeFS.ReadFile(localesDir + "/" + file.Name())
		if err != nil {
			return nil, err
		}

		_, err = bundle.ParseMessageFileBytes(data, file.Name())
		if err != nil {
			return nil, err
		}
	}

	return &Localizer{
		bundle:      bundle,
		currentLang: lang,
	}, nil
}

func (s *Localizer) Localize(messageID string, data map[string]any) string {
	localizer := i18n.NewLocalizer(s.bundle, s.currentLang.String())
	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: data,
	})
	if err != nil {
		return messageID
	}
	return msg
}
