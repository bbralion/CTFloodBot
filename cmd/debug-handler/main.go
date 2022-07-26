package main

import (
	"github.com/bbralion/CTFloodBot/pkg/handlers"
	"github.com/bbralion/CTFloodBot/pkg/utils"
	telegramapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jinzhu/configor"
	"go.uber.org/zap"
)

var config handlers.HandlerConfig

func main() {
	logger := utils.GetLogger()

	err := configor.Load(&config, "config_handler.yaml")
	if err != nil {
		logger.Fatal("Failed to parse app config", zap.Error(err))
	}

	handler := handlers.SimpleHandler{
		Handler: func(logger *zap.Logger, update *telegramapi.Update, _ handlers.AnswerChan) {
			logger.Info("Received update", zap.Any("update", update))
		},
		Logger: logger,
		Config: config,
	}
	handler.Run()
}
