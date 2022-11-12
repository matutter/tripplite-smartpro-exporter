package tripplite

import (
	"testing"

	"github.com/ilyakaznacheev/cleanenv"
	"github.com/rs/zerolog/log"
)

type AAA struct {
	A string
}

type BBB struct {
	AAA
	B string
}

func TestWatcher(t *testing.T) {

	type UPSMetricsTest struct {
		m      UPSMetrics
		expect bool
	}

	a := BBB{}
	a.A = "asd"
	a.B = "asdasd"

	w := Watcher{Scripts: map[string]*WatcherScript{}}

	script := Script{
		Name:           "test1",
		Charge:         20.0,
		Status:         "ob",
		ShutdownScript: "echo shutting down",
		CancelScript:   "echo shutdown cancelled",
	}

	w.AddScript(script, true)

	metrics := []UPSMetricsTest{
		{m: UPSMetrics{Status: "OB", BatteryCharge: 99.0}, expect: false},
		{m: UPSMetrics{Status: "OB", BatteryCharge: 80.0}, expect: false},
		{m: UPSMetrics{Status: "OB", BatteryCharge: 50.0}, expect: false},
		{m: UPSMetrics{Status: "OB", BatteryCharge: 30.0}, expect: false},
		{m: UPSMetrics{Status: "OB", BatteryCharge: 20.0}, expect: false},
		{m: UPSMetrics{Status: "OB", BatteryCharge: 19.999}, expect: true},
		{m: UPSMetrics{Status: "OB", BatteryCharge: 10.0}, expect: true},
		{m: UPSMetrics{Status: "OL", BatteryCharge: 15.0}, expect: false},
	}

	for _, m := range metrics {
		result := w.OnMetrics(&m.m)

		if m.expect != result {
			log.Error().Interface("metrics", m).Msgf("expected: %v, result: %v, charge: %f", m.expect, result, m.m.BatteryCharge)
			t.Fail()
		}
	}

}

func TestScriptDefaults(t *testing.T) {
	script := Script{}
	cleanenv.ReadEnv(&script)
	t.Logf("%v", script)
}
