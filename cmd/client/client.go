package main

import (
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/matutter/tripplite/pkg/tripplite"
	"github.com/rs/zerolog/log"
)

const (
	ACTION_FETCH_METRICS = "fetch-metrics"
	ACTION_UPDATE_CONFIG = "update-config"
)

type Client struct {
	s        Settings
	changeId string
	stale    bool
	secret   []byte
	running  bool
	actions  chan string
}

func NewClientFromSettings(s Settings) *Client {
	return &Client{
		s:        s,
		changeId: "",
		stale:    true,
		secret:   []byte(s.Secret),
		running:  false,
		actions:  make(chan string),
	}
}

func (c Client) AutoConfigEnabled() bool {
	return c.s.Autoconfigure
}

func (c Client) HMACEnabled() bool {
	return len(c.s.Secret) > 0
}

func (c Client) GetSecret() []byte {
	return c.secret
}

func (c *Client) SetChangeId(newConfigHash string) {
	c.stale = !strings.EqualFold(newConfigHash, c.changeId)
	c.changeId = newConfigHash
	if c.stale {
		log.Debug().Msg("config is stale")
	}
}

func (c Client) IsStale() bool {
	return c.stale
}

func (c Client) Url(parts ...string) string {
	sep := "/"
	url := c.s.Url
	for _, part := range parts {
		url = strings.TrimRight(url, sep) + sep + part
	}
	return strings.TrimRight(url, sep)
}

func (c *Client) FetchRemoteConfig() (*tripplite.PublicConfig, error) {
	conf := tripplite.PublicConfig{}
	err := Get(c, c.Url("config"), &conf)
	if err != nil {
		return nil, err
	}

	return &conf, err
}

func (c *Client) UpdateConfigFromRemote() error {

	conf, err := c.FetchRemoteConfig()
	if err != nil {
		return err
	}

	w := tripplite.Watcher{}

	if c.stale {
		delay, err := time.ParseDuration(conf.Delay)
		if err != nil {
			return err
		}
		c.s.Delay = delay
		for _, script := range c.s.Scripts {
			w.AddPublicScript(script)
		}
		c.stale = false
	}

	log.Info().Str("change_id", c.changeId).Msg("config up to date")

	return nil
}

func (c *Client) startUpdateTimer() {

	if len(c.changeId) == 0 && c.HMACEnabled() {
		c.actions <- ACTION_UPDATE_CONFIG
	}

	for c.running {
		time.Sleep(c.s.Delay)
		c.actions <- ACTION_FETCH_METRICS
	}

}

func (c *Client) Start() {

	if c.running {
		return
	}

	var sig os.Signal
	var action string
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	go c.startUpdateTimer()

	c.running = true

	for c.running {
		select {
		case sig = <-signals:
			log.Info().Str("signal", sig.String()).Msg("recieved keyboard interrupt, shutting down")
			c.running = false
		case action = <-c.actions:
			log.Debug().Msgf("running %s action", action)
			switch action {
			case ACTION_FETCH_METRICS:
				// TODO fetch metrics
				// TODO update watcher
			case ACTION_UPDATE_CONFIG:
				err := c.UpdateConfigFromRemote()
				if err != nil {
					log.Error().Err(err).Str("server", c.Url()).Msg("failed to update config")
				}
			}
		}
	}

	c.running = false
}
