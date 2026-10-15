// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"strings"
	"testing"
)

func TestDetailLines(t *testing.T) {
	useColor = false
	u := &Unit{Name: "a.service", Kind: "service", Description: "A thing", Load: "loaded", Active: "failed", Sub: "failed",
		Result: "exit-code", FileState: "enabled", Fragment: "/etc/systemd/system/a.service",
		DropIns: []string{"/etc/systemd/system/a.service.d/svcman.conf"}, User: "app",
		Exec: []ExecCmd{{Key: "ExecStart", Path: "/opt/a", Argv: "/opt/a --x"}}}
	h := &Host{r: &fakeRunner{}, Units: []*Unit{u}}
	a := &app{h: h, facts: map[string]*UnitFacts{"a.service": {Facts: []Fact{{Req: probeReq{Unit: "a.service", Role: "ExecStart", Kind: 'x', Path: "/opt/a", User: "app"}, State: "missing"}}}}}
	lines, fixes := a.detailLines(u, 100)
	txt := strings.Join(lines, "\n")
	for _, want := range []string{"a.service — A thing", "failed (exit-code)", "/etc/systemd/system/a.service",
		"svcman.conf", "ExecStart  /opt/a --x", "User       app", "[1] systemctl reset-failed a.service && systemctl restart a.service",
		"ExecStart /opt/a does not exist"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in\n%s", want, txt)
		}
	}
	if len(fixes) != 1 {
		t.Errorf("fixes %+v", fixes)
	}
}
