package main

import (
	"context"
	"encoding/json"

	"github.com/bbralion/CTFloodBot/pkg/core"
	"github.com/bbralion/CTFloodBot/pkg/utils"
	telegramapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/jinzhu/configor"
	"go.uber.org/zap"
)

var config core.BotCoreConfig

func main() {
	logger := utils.GetLogger()
	ctx := context.Background()

	err := configor.Load(&config, "config_core.yaml")
	if err != nil {
		logger.Fatal("Failed to parse app config", zap.Error(err))
	}

	botAPI, err := telegramapi.NewBotAPI(config.TelegramToken)
	if err != nil {
		logger.Fatal("Failed to create telegram api", zap.Error(err))
	}

	redisClient := core.GetRedisClientByConfig(config.Redis)

	tgUpdatesChan, err := botAPI.GetUpdatesChan(telegramapi.NewUpdate(0))
	if err != nil {
		logger.Fatal("Failed to get telegram updates chanel", zap.Error(err))
	}

	answerSubscriber := redisClient.Subscribe(ctx, core.RedisAnswersChanel)
	answerChan := answerSubscriber.Channel()
	for {
		select {
		case update := <-tgUpdatesChan:
			logger.Info("New update", zap.String("update_type", utils.GetTelegramUpdateType(&update)))
			byteMessage, err := json.Marshal(update)
			if err != nil {
				logger.Error("Failed to marshal telegram update", zap.Error(err))
				continue
			}

			command := redisClient.Publish(ctx, core.RedisUpdateChanel, byteMessage)
			if command.Err() != nil {
				logger.Error("Failed to send telegram update in redis", zap.Error(err))
			}

		case answerSubscriber := <-answerChan:
			message := answerSubscriber.Payload
			var answer core.HandlerAnswer
			err := json.Unmarshal([]byte(message), &answer)
			if err != nil {
				logger.Error("Failed to unmarshal handler answer", zap.Error(err), zap.String("answer", message))
				continue
			}
			var telegramSend telegramapi.Chattable
			switch answer.Type {
			case "message:message_config":
				telegramSend = answer.MessageConfig
			case "message:sticker_config":
				telegramSend = answer.StickerConfig
			}
			if telegramSend != nil {
				_, err = botAPI.Send(telegramSend)
				if err != nil {
					logger.Error("Failed to send in telegram", zap.String("type", answer.Type), zap.Error(err))
				}
			}
		}
	}
}
