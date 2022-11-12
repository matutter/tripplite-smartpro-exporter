package main

import (
	"github.com/rs/zerolog/log"
)

func main() {
	s, err := NewSettings(true)
	if err != nil || s == nil {
		log.Fatal().Err(err).Msg("cannot load configuration")
	}

	client := NewClientFromSettings(*s)
	client.Start()
}
