package tripplite

import (
	"io"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

type WatchScript struct {
	Name           string  `yaml:"name"`
	Charge         float64 `yaml:"charge" env-default:"20.0"`
	Status         string  `yaml:"status" env-default:"OB"`
	ShutdownScript string  `yaml:"script"`
	CancelScript   string  `yaml:"cancel"`
	Shell          string  `yaml:"shell" env-default:"/bin/sh"`
	Shared         bool    `yaml:"shared"`
	RemoteOnly     bool    `yaml:"remote_only"`
}

func (w WatchScript) getCharge() float64 {
	v := float64(w.Charge)
	if v <= 0 {
		return 0.0
	}
	if v >= 100 {
		return 100.0
	}
	return v
}

func (w WatchScript) Check(metrics UPSMetrics) bool {
	// Generally the staus must be "OB" (on battery) and...
	if strings.EqualFold(metrics.Status, w.Status) {
		// the charge needs to fall below the user-defined charge
		if metrics.BatteryCharge < w.getCharge() {
			return true
		}
	}
	return false
}

type WatchScriptStatus struct {
	Script  WatchScript
	Active  bool
	Running bool
}

func (w *WatchScriptStatus) Run(do_cancel bool) error {
	if w.Running {
		return nil
	}
	w.Running = true

	script := w.Script.ShutdownScript
	if do_cancel {
		script = w.Script.CancelScript
	}

	log.Info().Str("script", w.Script.Name).Str("exec", script).Msg("running")

	shell := exec.Command(w.Script.Shell, "-")
	stdin, err := shell.StdinPipe()
	if err == nil {
		defer stdin.Close()

		err = shell.Start()
		if err == nil {
			io.WriteString(stdin, script+"\n")
			err = shell.Wait()
			log.Info().Str("script", w.Script.Name).Int("exit", shell.ProcessState.ExitCode()).Msg("script complete")
		}
	}

	w.Running = false
	return err
}

type Watcher struct {
	scripts []*WatchScriptStatus
}

func (w *Watcher) AddScript(script WatchScript) {
	wst := WatchScriptStatus{Script: script, Active: false}
	if w.scripts == nil {
		w.scripts = []*WatchScriptStatus{&wst}
	} else {
		w.scripts = append(w.scripts, &wst)
	}
}

func (w Watcher) GetSize() int {
	if w.scripts == nil {
		return 0
	}
	return len(w.scripts)
}

func (w *Watcher) OnMetrics(m *UPSMetrics) {
	if w.scripts == nil {
		return
	}
	for _, wst := range w.scripts {
		if wst.Script.RemoteOnly {
			continue
		}

		active := wst.Script.Check(*m)
		if !wst.Active && active {
			log.Info().
				Str("script", wst.Script.Name).
				Float64("charge", wst.Script.getCharge()).
				Bool("active", active).
				Msg("state changed to active")

			go wst.Run(active)
		} else if wst.Active && !active {
			log.Info().
				Str("script", wst.Script.Name).
				Float64("charge", wst.Script.getCharge()).
				Bool("active", active).
				Msg("state changed from active to inactive")
			go wst.Run(active)
		}
		wst.Active = active
	}
}
