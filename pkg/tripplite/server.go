package tripplite

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"

	"github.com/rs/zerolog/log"
)

func parseIntQuery(r *http.Request, arg string, defaultVal int, max int, min int) int {
	str := r.URL.Query().Get(arg)
	if len(str) == 0 {
		return defaultVal
	}
	tmp, err := strconv.Atoi(str)
	if err != nil {
		return defaultVal
	}
	if tmp < min {
		tmp = min
	}
	if tmp > max {
		tmp = max
	}
	return tmp
}

type UPSMetricsListener interface {
	OnMetrics(*UPSMetrics)
}

type HttpApp struct {
	History     []*UPSMetrics
	Limit       int
	LastError   error
	Server      *http.Server
	Delay       time.Duration
	Secret      []byte
	HMACEnabled bool
	Listeners   []UPSMetricsListener
}

func NewHttpApp(limit int, delay time.Duration, secret string) *HttpApp {
	if limit < 1 {
		limit = 1
	}
	m := HttpApp{
		History:     make([]*UPSMetrics, 0, limit),
		Limit:       limit,
		LastError:   nil,
		Server:      nil,
		Delay:       delay,
		Secret:      []byte(secret),
		HMACEnabled: bool(len(secret) != 0),
		Listeners:   []UPSMetricsListener{},
	}
	return &m
}

func (h *HttpApp) ValidateHMAC(msg []byte, expectedMACB64 string) (bool, error) {
	if len(msg) == 0 || len(expectedMACB64) == 0 {
		return false, fmt.Errorf("empty raw message or epxected HMAC")
	}
	mac := hmac.New(sha256.New, h.Secret).Sum(msg)
	expectedMAC, err := base64.RawStdEncoding.DecodeString(expectedMACB64)
	if err != nil {
		return false, err
	}
	return hmac.Equal(mac, expectedMAC), nil
}

func (h *HttpApp) setHMACHeaders(msg []byte, w http.ResponseWriter) {
	mac := hmac.New(sha256.New, h.Secret).Sum(msg)
	w.Header().Add("X-Content-Hash", base64.RawStdEncoding.EncodeToString(mac))
}

func (h *HttpApp) sendJSON(o interface{}, w http.ResponseWriter) {
	if o == nil {
		w.WriteHeader(http.StatusNoContent)
	} else {
		w.Header().Set("Content-Type", "application/json")
		data, err := json.Marshal(o)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			if h.HMACEnabled {
				h.setHMACHeaders(data, w)
			}
			w.Write(data)
			w.WriteHeader(http.StatusOK)
		}
	}
}

func (h *HttpApp) Middleware(methods []string, handler http.HandlerFunc) http.HandlerFunc {

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

func (h *HttpApp) LatestMetrics() *UPSMetrics {
	l := len(h.History)
	if l == 0 {
		return nil
	}
	return h.History[l-1]
}

func (h *HttpApp) appendMetrics(m *UPSMetrics) {
	if len(h.History) >= h.Limit {
		h.History = append(h.History[1:], m)
	}
	h.History = append(h.History, m)

	for _, listener := range h.Listeners {
		listener.OnMetrics(m)
	}
}

func (h *HttpApp) StartServer(addr string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/metrics", http.StatusMovedPermanently)
	})

	mux.HandleFunc("/metrics", h.Middleware([]string{http.MethodGet}, func(w http.ResponseWriter, r *http.Request) {
		m := h.LatestMetrics()
		h.sendJSON(m, w)
	}))

	mux.HandleFunc("/history", h.Middleware([]string{http.MethodGet}, func(w http.ResponseWriter, r *http.Request) {
		l := len(h.History)
		limit := parseIntQuery(r, "limit", h.Limit, l, 0)
		h.sendJSON(h.History[:limit], w)
	}))

	mux.HandleFunc("/config", h.Middleware([]string{http.MethodGet}, func(w http.ResponseWriter, r *http.Request) {

		scripts := []WatchScript{}
		for _, listener := range h.Listeners {
			switch t := listener.(type) {
			case *Watcher:
				w := listener.(*Watcher)
				for _, script := range w.scripts {
					if script.Script.Shared || script.Script.RemoteOnly {
						scripts = append(scripts, script.Script)
					}
				}
			default:
				log.Debug().Msgf("unknown listener type: %T", t)
			}
		}

		conf := map[string]interface{}{
			"scripts": scripts,
			"delay":   h.Delay.String(),
		}

		h.sendJSON(conf, w)
	}))

	h.Server = &http.Server{Addr: addr, Handler: mux}

	log.Info().Str("address", addr).Msg("listening for requests")
	if err := h.Server.ListenAndServe(); err != nil {
		if err == http.ErrServerClosed {
			log.Info().Msg("server has shutdown")
		} else {
			log.Fatal().Err(err).Msg("server has shutdown unexpectedly")
		}
	}

}

func (h *HttpApp) StopServer() {
	server := h.Server
	h.Server = nil
	if server != nil {
		go func() {
			if err := server.Shutdown(context.Background()); err != nil {
				log.Fatal().Err(err).Msg("fatal error while shutting down server")
			}
		}()
	}
}

func (h *HttpApp) PollMetrics(mon *SmartProUPSMonitor) {

	var err error
	var sig os.Signal
	var m *UPSMetrics

	metrics, errors := mon.OpenStream(h.Delay)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt)

	for mon.streaming {
		select {
		case sig = <-signals:
			log.Info().Str("signal", sig.String()).Msg("recieved keyboard interrupt")
			mon.closeStream()
			h.StopServer()
		case err = <-errors:
			log.Error().Err(err).Msg("error gathering metrics")
		case m = <-metrics:
			log.Info().Interface("metrics", m).Send()
			h.appendMetrics(m)
		}
	}

}
