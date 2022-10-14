package main

import (
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func init() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	zerolog.SetGlobalLevel(zerolog.DebugLevel)
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
}

func main() {
	var vid uint16 = 0x09ae
	var pid uint16 = 0x0001

	h := NewHttpApp(1000)
	mon, err := NewSmartProUPSMonitor(vid, pid)
	if err != nil {
		log.Error().Err(err).Msg("failed to open monitor")
	} else {
		log.Info().
			Str("manufacturer", mon.manufacturer).
			Str("product", mon.product).
			Str("protocol", mon.protocolName).
			Msg("serving")

		go h.startServer("0.0.0.0:8080")
		h.pollMetrics(mon) // blocks until SIGTERM
		mon.Close()
	}
}
