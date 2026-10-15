// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"strings"
	"testing"
)

func names(rs []row) string {
	var s []string
	for _, r := range rs {
		n := r.u.Name
		if r.child {
			n = "  " + n
		}
		s = append(s, n)
	}
	return strings.Join(s, "|")
}

func testHost() *Host {
	return &Host{Units: []*Unit{
		{Name: "b.service", Kind: "service", Active: "active", Sub: "running", FileState: "enabled"},
		{Name: "a.service", Kind: "service", Active: "inactive", FileState: "disabled"},
		{Name: "z.service", Kind: "service", Active: "failed", FileState: "enabled"},
		{Name: "job.timer", Kind: "timer", Active: "active", FileState: "enabled", Triggers: []string{"job.service"}},
		{Name: "job.service", Kind: "service", Active: "inactive", FileState: "static", TriggeredBy: []string{"job.timer"}, Result: "exit-code"},
	}}
}

func TestBuildRowsOrderAndChildren(t *testing.T) {
	got := names(buildRows(testHost(), 0, ""))
	want := "z.service|a.service|b.service|job.timer|  job.service"
	if got != want {
		t.Errorf("got %s want %s", got, want)
	}
}

func TestBuildRowsFilters(t *testing.T) {
	h := testHost()
	for f, want := range map[int]string{
		1: "z.service|job.timer|  job.service", // failed: failed units, and timers whose service last failed
		2: "b.service",
		3: "z.service|b.service|job.timer|  job.service",
		4: "job.timer|  job.service",
	} {
		if got := names(buildRows(h, f, "")); got != want {
			t.Errorf("filter %s: got %s want %s", filters[f], got, want)
		}
	}
	if got := names(buildRows(h, 0, "job")); got != "job.timer|  job.service" {
		t.Errorf("search: %s", got)
	}
}

func TestCheckCell(t *testing.T) {
	useColor = false
	h := testHost()
	a := &app{h: h, facts: map[string]*UnitFacts{"b.service": {}}}
	if c := a.checkCell(h.Unit("a.service")); c != "·" {
		t.Errorf("unprobed: %q", c)
	}
	if c := a.checkCell(h.Unit("b.service")); c != "ok" {
		t.Errorf("ok: %q", c)
	}
	a.facts["z.service"] = &UnitFacts{}
	if c := a.checkCell(h.Unit("z.service")); c != "✗ 1" {
		t.Errorf("failed: %q", c)
	}
}

// Units that are only referenced and do not exist clutter the list; under a
// timer that triggers them they stay visible (that is the timer's problem).
func TestBuildRowsHidesNotFound(t *testing.T) {
	h := &Host{Units: []*Unit{
		{Name: "NetworkManager.service", Kind: "service", Load: "not-found"},
		{Name: "a.service", Kind: "service", Load: "loaded"},
		{Name: "o.timer", Kind: "timer", Load: "loaded", Triggers: []string{"ghost.service"}},
		{Name: "ghost.service", Kind: "service", Load: "not-found"},
	}}
	if got := names(buildRows(h, 0, "")); got != "a.service|o.timer|  ghost.service" {
		t.Errorf("%s", got)
	}
}

// Columns line up whatever the sub-state ("auto-restart" is the longest).
func TestRowLineColumnsAlign(t *testing.T) {
	useColor = false
	a := &app{h: &Host{}, facts: map[string]*UnitFacts{}}
	l1 := a.rowLine(row{u: &Unit{Name: "a.service", Kind: "service", Active: "activating", Sub: "auto-restart", FileState: "enabled"}})
	l2 := a.rowLine(row{u: &Unit{Name: "b.service", Kind: "service", Active: "active", Sub: "running", FileState: "enabled"}})
	if strings.Index(l1, "enabled") != strings.Index(l2, "enabled") {
		t.Errorf("misaligned:\n%q\n%q", l1, l2)
	}
}
