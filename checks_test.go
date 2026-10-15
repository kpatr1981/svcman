// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"strings"
	"testing"
	"time"
)

func has(is []Issue, sev Severity, text string) bool {
	for _, i := range is {
		if i.Sev == sev && strings.Contains(i.Text, text) {
			return true
		}
	}
	return false
}

func TestCheckProps(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	svc := &Unit{Name: "b.service", Kind: "service", Load: "loaded", Active: "failed", Result: "exit-code", FileState: "static"}
	h := &Host{Now: now, Units: []*Unit{
		{Name: "a.service", Kind: "service", Load: "not-found"},
		svc,
		{Name: "c.service", Kind: "service", Load: "loaded", Active: "active", NRestarts: 6},
		{Name: "d.service", Kind: "service", Load: "loaded", Active: "inactive", FileState: "enabled", Type: "simple"},
		{Name: "e.service", Kind: "service", Load: "loaded", Active: "inactive", FileState: "enabled", Type: "oneshot"},
		{Name: "f.service", Kind: "service", Load: "loaded", Active: "active", NeedReload: true},
		{Name: "g.timer", Kind: "timer", Load: "loaded", Active: "active", Triggers: []string{"ghost.service"}},
		{Name: "b.timer", Kind: "timer", Load: "loaded", Active: "active", Triggers: []string{"b.service"}, NextElapse: now.Add(-time.Hour)},
		{Name: "m.service", Kind: "service", Load: "masked"},
	}}
	cases := []struct {
		unit string
		sev  Severity
		text string
	}{
		{"a.service", SevFail, "does not load"},
		{"b.service", SevFail, "failed (exit-code)"},
		{"c.service", SevWarn, "restarted 6 times"},
		{"d.service", SevWarn, "enabled but not running"},
		{"f.service", SevWarn, "daemon-reload"},
		{"g.timer", SevFail, "ghost.service, which does not exist"},
		{"b.timer", SevWarn, "last run of b.service failed"},
		{"b.timer", SevWarn, "overdue"},
		{"m.service", SevInfo, "masked"},
	}
	for _, c := range cases {
		is := Check(h, h.Unit(c.unit), nil)
		if !has(is, c.sev, c.text) {
			t.Errorf("%s: want %v %q, got %+v", c.unit, c.sev, c.text, is)
		}
	}
	if is := Check(h, h.Unit("e.service"), nil); len(is) != 0 {
		t.Errorf("oneshot that ran must not warn: %+v", is)
	}
	is := Check(h, svc, nil)
	if len(is[0].Fixes) != 1 || is[0].Fixes[0].Cmd != "systemctl reset-failed b.service && systemctl restart b.service" {
		t.Errorf("fix %+v", is[0].Fixes)
	}
}
