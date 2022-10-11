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

func run_monitor(mon *SmartProUPSMonitor) {

	var err error

	metrics, err := mon.GetStats()
	if err != nil {
		log.Error().Err(err).Msg("get metrics error")
	}
	log.Info().
		Interface("metrics", metrics).
		Send()

}

func main() {

	var vid uint16 = 0x09ae
	var pid uint16 = 0x0001
	m, err := NewSmartProUPSMonitor(vid, pid)
	if err != nil {
		log.Error().Err(err).Msg("failed to open monitor")
	} else {
		log.Info().
			Str("manufacturer", m.manufacturer).
			Str("product", m.product).
			Str("protocol", m.protocolName).
			Send()

		run_monitor(m)

		m.Close()
	}
}
