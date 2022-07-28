package models

import (
	"regexp"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type (
	UpdateChan tgbotapi.UpdatesChannel
	ErrorChan  <-chan error
)

type MatcherGroup []*regexp.Regexp

func (g MatcherGroup) MatchString(s string) bool {
	for _, m := range g {
		if m.MatchString(s) {
			return true
		}
	}
	return false
}
