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
	Url           string                  `yaml:"host" env:"UPS_HOST" env-default:"http://127.0.0.1:8080"`
	Debug         bool                    `yaml:"debug" env:"UPS_DEBUG"`
	Secret        string                  `yaml:"secret" env:"UPS_HMAC_SECRET"`
	Delay         time.Duration           `yaml:"delay" env:"UPS_DELAY" env-default:"5s"`
	Autoconfigure bool                    `yaml:"auto_configure" env:"UPS_AUTOCONF"`
	Scripts       []tripplite.WatchScript `yaml:"scripts"`
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

	// why doesn't this happen automatically?
	for _, script := range s.Scripts {
		cleanenv.ReadEnv(&script)
	}

	return &s, err
}

var settings *Settings
var watcher *tripplite.Watcher

func init() {
	s, err := NewSettings(true)
	if err != nil {
		log.Fatal().Err(err).Msg("cannot load configuration")
	}

	w := tripplite.Watcher{}
	for _, script := range s.Scripts {
		w.AddScript(script)
	}

	if w.GetSize() > 0 {
		watcher = &w
	}

	settings = s
}

func main() {
	if settings == nil {
		os.Exit(1)
	}

	h := tripplite.NewHttpApp(0, settings.Delay, settings.Secret)

	if watcher != nil {
		h.Listeners = append(h.Listeners, watcher)
	}

	h.StartClient(settings.Url, settings.Autoconfigure)
}
