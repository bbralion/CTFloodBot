package main

import (
	"context"
	"log"
	"time"

	"github.com/bbralion/CTFloodBot/pkg/services"
)

func main() {
	streamer, err := services.NewLongPollStreamer(
		"https://api.telegram.org", "5417655434:AAH18hyT_Jz1GSD0ITS1kI7_oBBiGLAbwXE",
		services.LongPollOptions{})
	if err != nil {
		log.Println(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*10)
	defer cancel()
	stream := streamer.Stream(ctx)
	for x := range stream.AsTgBotAPI(services.DefaultCapacity) {
		log.Println(x.Error, x.Value.Message.Text)
	}
}
