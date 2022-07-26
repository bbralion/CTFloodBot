package utils

import (
	"strings"

	telegramapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

func addIfNotNil(slice []string, name string, objectIsNil bool) []string {
	if !objectIsNil {
		return append(slice, name)
	}
	return slice
}

func GetTelegramUpdateType(update *telegramapi.Update) string {
	var contents []string
	contents = addIfNotNil(contents, "message", update.Message == nil)
	contents = addIfNotNil(contents, "edited_message", update.EditedMessage == nil)
	contents = addIfNotNil(contents, "channel_post", update.ChannelPost == nil)
	contents = addIfNotNil(contents, "edited_channel_post", update.EditedChannelPost == nil)
	contents = addIfNotNil(contents, "inline_query", update.InlineQuery == nil)
	contents = addIfNotNil(contents, "chosen_inline_result", update.ChosenInlineResult == nil)
	contents = addIfNotNil(contents, "callback_query", update.CallbackQuery == nil)
	contents = addIfNotNil(contents, "shipping_query", update.ShippingQuery == nil)
	contents = addIfNotNil(contents, "pre_checkout_query", update.PreCheckoutQuery == nil)
	return strings.Join(contents, ",")
}
