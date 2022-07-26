package services

import "time"

type RegisterHandlerRequest struct {
	Name     string   `json:"name"`
	Matchers []string `json:"matchers"`
}

type RegisterHandlerResponse struct {
	// Must be consistent on renewals
	Channel string `json:"channel"`
	// Deadline is expected to be at least 30 seconds from now
	Deadline time.Time `json:"deadline"`
}
