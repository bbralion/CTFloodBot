package services

import (
	"fmt"

	"github.com/bbralion/CTFloodBot/pkg/models"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

// UpdateProvider represents a telegram update provider
type UpdateProvider interface {
	Updates() (models.UpdateChan, error)
}

const DefaultLongPollTimeoutS = 60

type pollingUpdateProvider struct {
	api *tgbotapi.BotAPI
}

func (p *pollingUpdateProvider) Updates() (models.UpdateChan, error) {
	updates, err := p.api.GetUpdatesChan(tgbotapi.UpdateConfig{
		Offset:  0,
		Limit:   p.api.Buffer,
		Timeout: DefaultLongPollTimeoutS,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get tgbotapi update channel: %w", err)
	}
	return models.UpdateChan(updates), nil
}

func NewPollingUpdateProvider(api *tgbotapi.BotAPI) UpdateProvider {
	return &pollingUpdateProvider{api}
}
