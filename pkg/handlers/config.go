package handlers

import (
	"github.com/kbats183/CTFloodBot/pkg/core"
)

type HandlerConfig struct {
	Redis core.BotRedisConfig `yaml:"redis"`
}
