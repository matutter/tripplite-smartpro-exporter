package tripplite

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

type Config struct {
	DelayString string        `json:"delay"`
	Scripts     []WatchScript `json:"scripts"`
	Delay       time.Duration
}

func (h *HttpApp) StartClient(url string, autoConfig bool) {

	// metrics_url := fmt.Sprintf("%s/%s", url, "metrics")

	if autoConfig {
		err := h.applyAutoConfig(url)
		if err != nil {
			log.Fatal().Err(err).Msg("unable to fetch upstream config")
		}
	}

	// TODO
	// 1. get metrics endpoint
	// 2. get metrics make sure data is not old
	// 3. pass to listeners

}

func (h *HttpApp) applyAutoConfig(base_url string) error {
	config := Config{}
	err := h.GetUpstreamConfig(base_url, &config)
	if err != nil {
		return err
	}

	watcher := Watcher{}
	for _, script := range config.Scripts {
		watcher.AddScript(script)
	}

	h.Listeners = []UPSMetricsListener{&watcher}
	h.Delay = config.Delay
	return nil
}

func (h *HttpApp) GetUpstreamConfig(base_url string, config *Config) error {
	config_url := fmt.Sprintf("%s/%s", base_url, "config")

	okay := true

	for retries := 10; retries > 0; retries-- {
		err := h.Get(config_url, &config)
		if err != nil {
			log.Error().Err(err).Str("url", config_url).Int("retries", retries).Msg("unable to get config")
			time.Sleep(5000 * time.Millisecond)
		} else {
			okay = true
			break
		}
	}

	if !okay {
		return fmt.Errorf("no more retries remain, unable to get config")
	}

	log.Info().Interface("config", config).Msg("recieved config ok")
	return nil
}

func (h *HttpApp) Get(url string, response interface{}) error {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}

	client := http.Client{Transport: transport}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	content_type := res.Header.Get("Content-Type")
	if content_type != "application/json" {
		return fmt.Errorf("expected Content-Type application/json, got %s instead", content_type)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	// validate HMAC
	if h.HMACEnabled {
		okay, err := h.ValidateHMAC(body, res.Header.Get("X-Content-Hash"))
		if err != nil {
			return err
		}
		if !okay {
			return fmt.Errorf("invalid HMAC for response from: %s", url)
		}
		log.Debug().Str("url", url).Msg("hmac OK")
	}

	err = json.Unmarshal(body, response)
	if err != nil {
		return err
	}

	switch response.(type) {
	case *Config:
		config := response.(*Config)
		config.Delay, err = time.ParseDuration(config.DelayString)
		if err != nil {
			return nil
		}
	}

	return nil
}
