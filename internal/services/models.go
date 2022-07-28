package services

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type (
	UpdateChan tgbotapi.UpdatesChannel
	ErrorChan  <-chan error
)
