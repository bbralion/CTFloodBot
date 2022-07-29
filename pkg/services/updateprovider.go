package services

import (
	"context"

	"github.com/go-logr/logr"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// UpdateProvider represents a telegram update provider
type UpdateProvider interface {
	Updates(ctx context.Context) tgbotapi.UpdatesChannel
}

const DefaultLongPollTimeoutS = 60

type pollingUpdateProvider struct {
	logger logr.Logger
	api    *tgbotapi.BotAPI
}

func (p *pollingUpdateProvider) Updates(ctx context.Context) tgbotapi.UpdatesChannel {
	updates, _ := p.api.GetUpdatesChan(tgbotapi.UpdateConfig{
		Offset:  0,
		Limit:   p.api.Buffer,
		Timeout: DefaultLongPollTimeoutS,
	})

	go func() {
		<-ctx.Done()
		p.api.StopReceivingUpdates()
	}()
	return updates
}

func NewPollingUpdateProvider(logger logr.Logger, api *tgbotapi.BotAPI) UpdateProvider {
	return &pollingUpdateProvider{logger, api}
}
