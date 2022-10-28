package tripplite

import (
	"testing"

	"github.com/ilyakaznacheev/cleanenv"
)

func TestWatcher(t *testing.T) {
	w := Watcher{scripts: []*WatchScriptStatus{}}

	st := WatchScript{
		Name:           "test1",
		Charge:         20.0,
		Status:         "ob",
		ShutdownScript: "echo shutting down",
		CancelScript:   "echo shutdown cancelled",
	}

	w.AddScript(st)

	metrics := []UPSMetrics{
		{Status: "OB", BatteryCharge: 99.0},
		{Status: "OB", BatteryCharge: 80.0},
		{Status: "OB", BatteryCharge: 50.0},
		{Status: "OB", BatteryCharge: 30.0},
		{Status: "OB", BatteryCharge: 20.0},
		{Status: "OB", BatteryCharge: 10.0},
		{Status: "OL", BatteryCharge: 15.0},
	}

	for _, m := range metrics {
		w.OnMetrics(&m)
	}

}

func TestScriptDefaults(t *testing.T) {
	script := WatchScript{}
	cleanenv.ReadEnv(&script)
	t.Logf("%v", script)
}
