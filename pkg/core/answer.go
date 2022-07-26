package core

import (
	telegramapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type HandlerAnswer struct {
	Type          string                     `json:"type"` // one of message:message_config, message:sticker_config
	MessageConfig *telegramapi.MessageConfig `json:"message_config"`
	StickerConfig *telegramapi.StickerConfig `json:"sticker_config"`
}
