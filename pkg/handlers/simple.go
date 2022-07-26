package handlers

import (
	"context"
	"encoding/json"
	telegramapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/kbats183/CTFloodBot/pkg/core"
	"go.uber.org/zap"
)

type AnswerChan chan<- core.HandlerAnswer

type SimpleHandler struct {
	Handler func(logger *zap.Logger, update *telegramapi.Update, answerChan AnswerChan)
	Logger  *zap.Logger
	Config  HandlerConfig
}

func (h *SimpleHandler) Run() {
	ctx := context.Background()

	redisClient := core.GetRedisClientByConfig(h.Config.Redis)

	publish := func(message interface{}) {
		command := redisClient.Publish(ctx, core.RedisAnswersChanel, message)
		if command.Err() != nil {
			h.Logger.Error("Failed to send answer", zap.Error(command.Err()))
		}
	}

	h.Logger.Info("Handler is ready to start")

	subscriber := redisClient.Subscribe(ctx, core.RedisUpdateChanel)
	for message := range subscriber.Channel() {
		var update telegramapi.Update
		err := json.Unmarshal([]byte(message.Payload), &update)
		if err != nil {
			h.Logger.Fatal("Failed to unmarshal received update", zap.Error(err), zap.String("message", message.Payload))
		}

		go h.processUpdate(&update, publish)
	}
}

func (h *SimpleHandler) createAnswerChan(publish func(message interface{})) AnswerChan {
	ch := make(chan core.HandlerAnswer)
	go func() {
		for v := range ch {
			marshal, err := json.Marshal(v)
			if err != nil {
				h.Logger.Error("Failed to marshal answer", zap.Error(err))
			}
			publish(marshal)
		}
	}()
	return ch
}

func (h *SimpleHandler) processUpdate(update *telegramapi.Update, publish func(message interface{})) {
	ch := h.createAnswerChan(publish)
	h.Handler(h.Logger, update, ch)
	close(ch)
}
