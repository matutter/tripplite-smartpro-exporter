package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/matutter/tripplite/pkg/tripplite"
	"github.com/rs/zerolog/log"
)

func Get(app tripplite.App, url string, response interface{}) error {
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
	if app.HMACEnabled() {
		okay, err := tripplite.ValidateHMAC(app, body, res.Header.Get(tripplite.HTTP_CONTENT_HASH_HEADER))
		if err != nil {
			return err
		}
		if !okay {
			return fmt.Errorf("invalid HMAC for response from: %s", url)
		}
		app.SetChangeId(res.Header.Get(tripplite.HTTP_CHANGE_ID_HEADER))
		log.Debug().Str("url", url).Msg("hmac OK")
	}

	err = json.Unmarshal(body, response)
	if err != nil {
		return err
	}

	return nil
}
