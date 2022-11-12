package tripplite

import (
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/rs/zerolog/log"
)

// From API endpoints
type PublicScript struct {
	Name           string  `json:"name"`
	Charge         float64 `json:"charge"`
	Status         string  `json:"status"`
	ShutdownScript string  `json:"script"`
	CancelScript   string  `json:"cancel"`
}

// From Configs
type Script struct {
	Public         bool    `json:"public" yaml:"public"`
	RemoteOnly     bool    `json:"remote_only" yaml:"remote_only"`
	Name           string  `json:"name" yaml:"name"`
	Charge         float64 `json:"charge" yaml:"charge"`
	Status         string  `json:"status" yaml:"status"`
	ShutdownScript string  `json:"script" yaml:"script"`
	CancelScript   string  `json:"cancel" yaml:"cancel"`
}

func (w Script) getCharge() float64 {
	v := float64(w.Charge)
	if v <= 0 {
		return 0.0
	}
	if v >= 100 {
		return 100.0
	}
	return v
}

func (w Script) Check(metrics UPSMetrics) bool {
	// Generally the staus must be "OB" (on battery) and...
	if strings.EqualFold(metrics.Status, w.Status) {
		// the charge needs to fall below the user-defined charge
		if metrics.BatteryCharge < w.getCharge() {
			return true
		}
	}
	return false
}

type WatcherScript struct {
	Script
	Active  bool
	Running bool
	Enabled bool
}

func (s WatcherScript) ToPublicScript() PublicScript {
	return PublicScript{
		Name:           s.Name,
		Charge:         s.Charge,
		Status:         s.Status,
		ShutdownScript: s.ShutdownScript,
		CancelScript:   s.CancelScript,
	}
}

func (w *WatcherScript) FromScript(s Script) *WatcherScript {
	w.Name = s.Name
	w.Charge = s.Charge
	w.Status = s.Status
	w.ShutdownScript = s.ShutdownScript
	w.CancelScript = s.CancelScript
	return w
}

func (w WatcherScript) GetShell() string {
	shell := os.Getenv("SHELL")
	if len(shell) == 0 {
		shell = "/bin/sh"
	}
	return shell
}

func (w *WatcherScript) Run(do_cancel bool) error {
	if w.Running {
		return nil
	}
	w.Running = true

	script := w.ShutdownScript
	if do_cancel {
		script = w.CancelScript
	}

	log.Info().Str("script", w.Name).Str("exec", script).Msg("running")

	shell_exec := w.GetShell()
	shell := exec.Command(shell_exec, "-")
	stdin, err := shell.StdinPipe()
	if err == nil {
		defer stdin.Close()

		err = shell.Start()
		if err == nil {
			io.WriteString(stdin, script+"\n")
			err = shell.Wait()
			log.Info().Str("script", w.Name).Int("exit", shell.ProcessState.ExitCode()).Msg("script complete")
		}
	}

	w.Running = false
	return err
}

type Watcher struct {
	Scripts map[string]*WatcherScript
}

func NewWatcher() *Watcher {
	w := Watcher{Scripts: map[string]*WatcherScript{}}
	return &w
}

func (w *Watcher) AddScript(script Script, enableRemote bool) {

	if w.Scripts == nil {
		log.Fatal().Msg("script map is nil")
		return
	}

	w.Scripts[strings.ToLower(script.Name)] = &WatcherScript{
		Script:  script,
		Active:  false,
		Running: false,
		Enabled: enableRemote || !script.RemoteOnly,
	}

	log.Info().Interface("script", script).Msgf("loaded script %s", script.Name)
}

func (w *Watcher) AddPublicScript(script PublicScript) {
	if w.Scripts == nil {
		log.Fatal().Msg("script map is nil")
		return
	}

	w.Scripts[strings.ToLower(script.Name)] = &WatcherScript{
		Script: Script{
			Public:         true,
			RemoteOnly:     false,
			Name:           script.Name,
			Charge:         script.Charge,
			Status:         script.Status,
			ShutdownScript: script.ShutdownScript,
			CancelScript:   script.CancelScript,
		},
		Active:  false,
		Running: false,
		Enabled: true,
	}
}

func (w *Watcher) DisableAll() {
	for _, script := range w.Scripts {
		script.Enabled = false
		if script.Active {
			script.Run(true)
		}
	}
}

func (w Watcher) GetSize() int {
	if w.Scripts == nil {
		return 0
	}
	return len(w.Scripts)
}

func (w *Watcher) OnMetrics(m *UPSMetrics) bool {
	if w.Scripts == nil {
		return false
	}

	any_active := false

	for _, wst := range w.Scripts {
		if !wst.Enabled {
			continue
		}

		active := wst.Script.Check(*m)
		if active {
			any_active = true
		}

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

	return any_active
}
