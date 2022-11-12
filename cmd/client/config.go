package main

import (
	"os"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/matutter/tripplite/pkg/tripplite"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Settings struct {
	Url           string                   `yaml:"host" env:"UPS_HOST" env-default:"http://127.0.0.1:8080"`
	Debug         bool                     `yaml:"debug" env:"UPS_DEBUG"`
	Secret        string                   `yaml:"secret" env:"UPS_HMAC_SECRET"`
	Delay         time.Duration            `yaml:"delay" env:"UPS_DELAY" env-default:"5s"`
	Autoconfigure bool                     `yaml:"auto_configure" env:"UPS_AUTOCONF"`
	Scripts       []tripplite.PublicScript `yaml:"scripts"`
}

func NewSettings(use_env bool) (*Settings, error) {
	s := Settings{}

	err := cleanenv.ReadEnv(s)
	if err != nil {
		log.Error().Err(err).Msg("invalid environment configuration")
		return nil, err
	}

	config_path := os.Getenv("UPS_CONFIG")
	if len(config_path) > 0 {
		err := cleanenv.ReadConfig(config_path, &s)
		if err != nil {
			log.Error().Err(err).Str("path", config_path).Msg("failed to load config file")
			return nil, err
		}
	}

	if s.Debug {
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	if len(config_path) > 0 {
		log.Info().Str("path", config_path).Interface("config", s).Msg("config loaded")
	}

	for _, script := range s.Scripts {
		cleanenv.ReadEnv(&script)
	}

	return &s, err
}
