package main

import (
	"os"

	"github.com/matutter/tripplite/pkg/tripplite"
	"github.com/rs/zerolog/log"
)

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

	vid, pid := settings.getVidPid()
	h := tripplite.NewHttpApp(settings.HistorySize, settings.Delay, settings.Secret)

	if watcher != nil {
		h.Listeners = append(h.Listeners, watcher)
	}

	mon, err := tripplite.NewSmartProUPSMonitor(vid, pid)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open monitor")
	} else {

		log.Info().
			Str("manufacturer", mon.Manufacturer).
			Str("product", mon.Product).
			Str("protocol", mon.ProtocolName).
			Dur("delay", h.Delay).
			Msg("serving")

		go h.StartServer(settings.Listen)
		h.PollMetrics(mon) // blocks until SIGTERM
		mon.Close()
	}
}
