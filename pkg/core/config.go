package core

import (
	"github.com/go-redis/redis/v8"
)

type BotRedisConfig struct {
	Host     string `yaml:"host" env:"DB_HOST"`
	Username string `yaml:"user" env:"DB_USER"`
	Password string `yaml:"password" env:"DB_PASSWORD"`
}

type BotCoreConfig struct {
	TelegramToken string         `yaml:"auth_token" env:"BOT_AUTH_TOKEN"`
	Redis         BotRedisConfig `yaml:"redis"`
}

func GetRedisClientByConfig(config BotRedisConfig) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     config.Host,
		Username: config.Username,
		Password: config.Password,
	})
}
