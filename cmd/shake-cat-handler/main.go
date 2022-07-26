package main

import (
	"strings"

	"github.com/bbralion/CTFloodBot/pkg/core"
	"github.com/bbralion/CTFloodBot/pkg/handlers"
	"github.com/bbralion/CTFloodBot/pkg/utils"
	telegramapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jinzhu/configor"
	"go.uber.org/zap"
)

var config handlers.HandlerConfig

const (
	TelegramTextCommand = "/shake_cat_stick"
	StickerID           = "CAACAgIAAxkBAAIBiGLfzvi09zcCIPcPc6pu4_GsC3nwAAJVHQACm9J4Sf-ATjduPn5eKQQ"
)

func main() {
	logger := utils.GetLogger()

	err := configor.Load(&config, "config_handler.yaml")
	if err != nil {
		logger.Fatal("Failed to parse app config", zap.Error(err))
	}

	handler := handlers.SimpleHandler{
		Handler: func(logger *zap.Logger, update *telegramapi.Update, answerChan handlers.AnswerChan) {
			message := update.Message
			if message == nil {
				return
			}
			if strings.HasPrefix(message.Text, TelegramTextCommand) {
				sticker := telegramapi.NewStickerShare(message.Chat.ID, StickerID)
				answerChan <- core.HandlerAnswer{Type: "message:sticker_config", StickerConfig: &sticker}
			}
		},
		Logger: logger,
		Config: config,
	}
	handler.Run()
}
