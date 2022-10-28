package main

import (
	"os"
	"strconv"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/matutter/tripplite/pkg/tripplite"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Settings struct {
	Listen      string                  `yaml:"listen" env:"UPS_LISTEN" env-default:"0.0.0.0:8080"`
	Debug       bool                    `yaml:"debug" env:"UPS_DEBUG"`
	VendorId    string                  `yaml:"vendor_id" env:"UPS_VENDOR_ID"`
	ProductId   string                  `yaml:"product_id" env:"UPS_PRODUCT_ID"`
	Delay       time.Duration           `yaml:"delay" env:"UPS_DELAY" env-default:"5s"`
	HistorySize int                     `yaml:"history_size" env:"UPS_HISTORY_SIZE" env-default:"1000"`
	Scripts     []tripplite.WatchScript `yaml:"scripts"`
	Secret      string                  `yaml:"secret" env:"UPS_HMAC_SECRET"`
}

func (s Settings) getVidPid() (uint16, uint16) {
	var vid uint16
	var pid uint16
	var tmp int64
	var err error
	tmp, err = strconv.ParseInt(s.VendorId, 16, 16)
	if err != nil {
		log.Fatal().Err(err).Str("vendor_id", s.VendorId).Msg("invalid vendor id")
	}
	vid = (uint16(tmp))
	tmp, err = strconv.ParseInt(s.ProductId, 16, 16)
	if err != nil {
		log.Fatal().Err(err).Str("product_id", s.ProductId).Msg("invalid product id")
	}
	pid = (uint16(tmp))
	return vid, pid
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
