package handlers

import (
	"github.com/bbralion/CTFloodBot/pkg/core"
)

type HandlerConfig struct {
	Redis core.BotRedisConfig `yaml:"redis"`
}
