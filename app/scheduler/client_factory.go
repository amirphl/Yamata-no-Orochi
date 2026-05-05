package scheduler

import "github.com/amirphl/Yamata-no-Orochi/config"

// NewBotClient creates a new bot API client instance.
func NewBotClient(cfg config.BotConfig) BotClient {
	return newHTTPBotClient(cfg)
}

// NewPayamSMSClient creates a new PayamSMS client instance.
func NewPayamSMSClient(cfg config.PayamSMSConfig) PayamSMSClient {
	return newHTTPPayamSMSClient(cfg)
}

// NewBaleClient creates a new Bale client instance.
func NewBaleClient(cfg config.BaleConfig) BaleClient {
	return newHTTPBaleClient(cfg)
}

// NewRubikaClient creates a new Rubika client instance.
func NewRubikaClient(cfg config.RubikaConfig) RubikaClient {
	return newHTTPRubikaClient(cfg)
}

// NewSplusClient creates a new Splus client instance.
func NewSplusClient(cfg config.SplusConfig) SplusClient {
	return newHTTPSplusClient(cfg)
}
