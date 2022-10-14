package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

func parseIntQuery(r *http.Request, arg string, defaultVal int) int {
	str := r.URL.Query().Get(arg)
	if len(str) == 0 {
		return defaultVal
	}

	tmp, err := strconv.Atoi(str)
	if err == nil {
		return tmp
	}

	return defaultVal
}

func simpleJSONMessage(key string, value string) []byte {
	m := map[string]string{key: value}
	data, err := json.Marshal(m)
	if err != nil {
		log.Error().Err(err).Send()
		return []byte{'{', '}'}
	}
	return data
}

func sendJSON(o interface{}, w http.ResponseWriter) {
	if o == nil {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.Header().Set("Content-Type", "application/json")
		err := json.NewEncoder(w).Encode(o)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}
}

func Middleware(methods []string, handler http.HandlerFunc) http.HandlerFunc {

	meth := map[string]bool{}
	for _, m := range methods {
		meth[m] = true
	}

	return func(w http.ResponseWriter, r *http.Request) {
		if _, ok := meth[r.Method]; !ok {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		handler(w, r)
	}
}

type HttpApp struct {
	history   []*UPSMetrics
	limit     int
	lastError error
	server    *http.Server
}

func NewHttpApp(limit int) *HttpApp {
	if limit < 1 {
		limit = 1
	}
	m := HttpApp{
		history:   make([]*UPSMetrics, 0, limit),
		limit:     limit,
		lastError: nil,
		server:    nil,
	}
	return &m
}

func (h *HttpApp) latestMetrics() *UPSMetrics {
	l := len(h.history)
	if l == 0 {
		return nil
	}
	return h.history[l-1]
}

func (h *HttpApp) appendMetrics(m *UPSMetrics) {
	if len(h.history) >= h.limit {
		h.history = append(h.history[1:], m)
	}
	h.history = append(h.history, m)
}

func (h *HttpApp) startServer(addr string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/metrics", http.StatusMovedPermanently)
	})

	mux.HandleFunc("/metrics", Middleware([]string{http.MethodGet}, func(w http.ResponseWriter, r *http.Request) {
		m := h.latestMetrics()
		sendJSON(m, w)
	}))

	mux.HandleFunc("/history", Middleware([]string{http.MethodGet}, func(w http.ResponseWriter, r *http.Request) {
		limit := parseIntQuery(r, "limit", h.limit)
		l := len(h.history)
		if limit >= l {
			limit = l
		}
		sendJSON(h.history[:limit], w)
	}))

	h.server = &http.Server{Addr: addr, Handler: mux}

	log.Info().Str("address", addr).Msg("listening for requests")
	if err := h.server.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			log.Info().Msg("server has shutdown")
		} else {
			log.Fatal().Err(err).Msg("server has shutdown unexpectedly")
		}
	}

}

func (h *HttpApp) stopServer() {
	server := h.server
	h.server = nil
	if server != nil {
		go func() {
			if err := server.Shutdown(context.Background()); err != nil {
				log.Fatal().Err(err).Msg("fatal error while shutting down server")
			}
		}()
	}
}

func (h *HttpApp) pollMetrics(mon *SmartProUPSMonitor) {

	var err error
	var sig os.Signal
	var m *UPSMetrics

	metrics, errors := mon.openStream(5000 * time.Millisecond)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	for mon.streaming {
		select {
		case sig = <-signals:
			log.Info().Str("signal", sig.String()).Msg("recieved keyboard interrupt")
			mon.closeStream()
			h.stopServer()
		case err = <-errors:
			log.Error().Err(err).Msg("error gathering metrics")
		case m = <-metrics:
			log.Info().Interface("metrics", m).Send()
			h.appendMetrics(m)
		}
	}

}
