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
func TestCheckFacts(t *testing.T) {
	u := &Unit{Name: "a.service", Kind: "service", Load: "loaded", Active: "active"}
	mk := func(kind byte, role, path string, opt bool, f Fact) Fact {
		f.Req = probeReq{Unit: "a.service", Role: role, Kind: kind, Path: path, User: "app", Optional: opt}
		return f
	}
	uf := &UnitFacts{Facts: []Fact{
		mk('x', "ExecStart", "/opt/nope", false, Fact{State: "missing"}),
		mk('x', "ExecStartPre", "/opt/pre", true, Fact{State: "missing"}),
		mk('x', "ExecStart", "/opt/run.sh", false, Fact{Exists: true, Type: "file", Mode: 0o644, Owner: "app", Access: "denied", Interp: "/bin/bash", InterpOK: true}),
		mk('x', "ExecStart", "/opt/c.sh", false, Fact{Exists: true, Type: "file", Mode: 0o755, Owner: "root", Access: "ok", Interp: "/bin/sh", InterpOK: true, CRLF: true}),
		mk('x', "ExecStart", "/opt/i.sh", false, Fact{Exists: true, Type: "file", Mode: 0o755, Owner: "root", Access: "ok", Interp: "/usr/bin/python9", InterpOK: false}),
		mk('x', "ExecStart", "/mnt/n.sh", false, Fact{Exists: true, Type: "file", Mode: 0o755, Owner: "root", Access: "ok", NoExec: true}),
		mk('d', "WorkingDirectory", "/srv/a", false, Fact{State: "missing"}),
		mk('r', "EnvironmentFile", "/etc/a.env", true, Fact{State: "missing"}),
		mk('f', "unit file", "/etc/systemd/system/a.service", false, Fact{Exists: true, Type: "file", Mode: 0o666, Owner: "root"}),
		mk('u', "User", "app", false, Fact{}),
		mk('s', "vendor", "/usr/lib/systemd/system/a.service", false, Fact{Exists: true}),
	}, Verify: []string{"/x:3: Unknown key name 'Foo'"}}
	is := checkFacts(u, uf)
	for _, w := range []struct {
		sev  Severity
		text string
	}{
		{SevFail, "ExecStart /opt/nope does not exist"},
		{SevWarn, "ExecStartPre /opt/pre does not exist"},
		{SevFail, "/opt/run.sh is not executable by app"},
		{SevFail, "Windows (CRLF) line endings"},
		{SevFail, "interpreter /usr/bin/python9 does not exist"},
		{SevFail, "noexec"},
		{SevFail, "WorkingDirectory /srv/a does not exist"},
		{SevFail, "writable by group or others"},
		{SevFail, "user app does not exist"},
		{SevInfo, "overrides the vendor unit"},
		{SevWarn, "Unknown key name 'Foo'"},
	} {
		if !has(is, w.sev, w.text) {
			t.Errorf("missing %v %q in %+v", w.sev, w.text, is)
		}
	}
	if has(is, SevFail, "/etc/a.env") || has(is, SevWarn, "/etc/a.env") {
		t.Error("optional missing EnvironmentFile must not be reported")
	}
	for _, i := range is {
		if strings.Contains(i.Text, "/opt/run.sh") && (len(i.Fixes) != 1 || i.Fixes[0].Cmd != "chmod u+x /opt/run.sh") {
			t.Errorf("chmod fix %+v", i.Fixes)
		}
		if strings.Contains(i.Text, "CRLF") && (len(i.Fixes) != 1 || i.Fixes[0].Cmd != `sed -i 's/\r$//' /opt/c.sh`) {
			t.Errorf("crlf fix %+v", i.Fixes)
		}
		if strings.Contains(i.Text, "writable") && i.Fixes[0].Cmd != "chmod go-w /etc/systemd/system/a.service" {
			t.Errorf("perm fix %+v", i.Fixes)
		}
	}
}

func TestChmodFixWho(t *testing.T) {
	f := Fact{Req: probeReq{Path: "/p", User: "bob"}, Mode: 0o750, Owner: "root"}
	if fx := chmodFix(f, "x"); fx == nil || fx.Cmd != "chmod o+x /p" {
		t.Errorf("other user: %+v", fx)
	}
	f.Req.User = "root"
	f.Mode = 0o644
	if fx := chmodFix(f, "x"); fx == nil || fx.Cmd != "chmod u+x /p" {
		t.Errorf("root: %+v", fx)
	}
	f.Mode = 0o744 // root can already execute: the problem is elsewhere (a parent directory)
	if fx := chmodFix(f, "x"); fx != nil {
		t.Errorf("want no fix, got %+v", fx)
	}
}
