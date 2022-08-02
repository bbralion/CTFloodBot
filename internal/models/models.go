package models

import (
	"regexp"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type PossibleUpdate struct {
	Update tgbotapi.Update
	Error  error
}

type UpdateChan <-chan PossibleUpdate

type MatcherGroup []*regexp.Regexp

func (g MatcherGroup) MatchString(s string) bool {
	for _, m := range g {
		if m.MatchString(s) {
			return true
		}
	}
	return false
}
