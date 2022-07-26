package main

import (
	"github.com/bbralion/CTFloodBot/pkg/core"
	"github.com/bbralion/CTFloodBot/pkg/handlers"
	"github.com/bbralion/CTFloodBot/pkg/utils"
	telegramapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jinzhu/configor"
	"go.uber.org/zap"
)

var config handlers.HandlerConfig

const (
	TelegramTextCommand = "aboba"
	TelegramTextAnswer  = "abobus"
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
			if message.Text == TelegramTextCommand {
				msg := telegramapi.NewMessage(message.Chat.ID, TelegramTextAnswer)
				msg.ReplyToMessageID = message.MessageID
				answerChan <- core.HandlerAnswer{Type: "message:message_config", MessageConfig: &msg}
			}
		},
		Logger: logger,
		Config: config,
	}
	handler.Run()
}
