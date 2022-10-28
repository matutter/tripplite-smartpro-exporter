package main

import (
	"testing"

	"github.com/ilyakaznacheev/cleanenv"
)

func TestLoadConfig(t *testing.T) {
	s := Settings{}
	cleanenv.ReadConfig("../../config/upsmon.yml", &s)

	t.Logf("vendor_id: %s, product_id: %s", s.VendorId, s.ProductId)
	vid, pid := s.getVidPid()
	t.Logf("vendor_id: %d, product_id: %d", vid, pid)
}

func TestLoadConfig2(t *testing.T) {
	s := Settings{}
	cleanenv.ReadConfig("../../config/upsmon.yml", &s)
	for _, script := range s.Scripts {
		cleanenv.ReadEnv(&script)
	}
	t.Logf("%v", s)
}
