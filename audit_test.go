// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"strings"
	"testing"
)

func TestAuditUnitsSelection(t *testing.T) {
	h := &Host{Units: []*Unit{
		{Name: "en.service", Kind: "service", FileState: "enabled", Load: "loaded"},
		{Name: "fail.service", Kind: "service", FileState: "static", Active: "failed", Load: "loaded"},
		{Name: "t.timer", Kind: "timer", FileState: "enabled", Load: "loaded", Triggers: []string{"job.service"}},
		{Name: "job.service", Kind: "service", FileState: "static", Load: "loaded", TriggeredBy: []string{"t.timer"}},
		{Name: "vendor.service", Kind: "service", FileState: "static", Load: "loaded", Active: "inactive"},
		{Name: "m.service", Kind: "service", Load: "masked", FileState: "masked"},
	}}
	var got []string
	for _, u := range auditUnits(h, false) {
		got = append(got, u.Name)
	}
	if strings.Join(got, ",") != "en.service,fail.service,job.service,t.timer" {
		t.Errorf("default: %v", got)
	}
	if n := len(auditUnits(h, true)); n != 6 {
		t.Errorf("-v: %d units", n)
	}
}

func TestHostIssues(t *testing.T) {
	h := &Host{SysState: "degraded", Units: []*Unit{
		{Name: "m.service", Load: "masked"},
		{Name: "a.service", Load: "loaded", FileState: "enabled", Wants: []string{"m.service"}},
		{Name: "b.service", Load: "loaded", Active: "failed"},
	}}
	is := hostIssues(h)
	if !has(is, SevWarn, "degraded") || !has(is, SevWarn, "m.service is masked but wanted by a.service") || !has(is, SevFail, "1 failed unit: b.service") {
		t.Errorf("%+v", is)
	}
}

func TestJournalCmd(t *testing.T) {
	if got := journalCmd("a b.service", 50, false); got != envC+" journalctl --no-pager -o short-iso -n 50 -u 'a b.service'" {
		t.Errorf("%q", got)
	}
	if !strings.HasSuffix(journalCmd("a.service", 50, true), " -f") {
		t.Error("follow")
	}
}

func TestJournalFallsBackToRoot(t *testing.T) {
	f := &fakeRunner{reply: func(cmd, in string) (string, string, error) {
		if strings.Contains(cmd, "sudo -n") {
			return "2026-10-15T01:00:00+0000 h a[1]: boom\n", "", nil
		}
		return "-- No entries --\n", "Hint: You are currently not seeing messages from other users and the system.\n", nil
	}}
	h := &Host{r: f, Priv: PrivNoPass}
	ls, err := h.journal("a.service", 10)
	if err != nil || len(ls) != 1 || !strings.Contains(ls[0], "boom") || !h.journalRoot {
		t.Errorf("%v %v %v", ls, err, h.journalRoot)
	}
}

// A unit that does not exist (only referenced by a timer) is reported by the
// timer's check, not as a unit of its own.
func TestAuditUnitsSkipsNotFound(t *testing.T) {
	h := &Host{Units: []*Unit{{Name: "ghost.service", Kind: "service", Load: "not-found", TriggeredBy: []string{"t.timer"}}}}
	if n := len(auditUnits(h, false)); n != 0 {
		t.Errorf("got %d", n)
	}
}

// Without a terminal (cron, CI, pipes) svcman must not wait for a password.
func TestAskPasswordWithoutTerminalDoesNotBlock(t *testing.T) {
	old := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }
	defer func() { stdinIsTerminal = old }()
	f := &fakeRunner{}
	h := &Host{r: f, Priv: PrivPassword}
	h.askPassword("pw: ")
	if len(f.calls) != 0 || h.canRoot() {
		t.Errorf("asked anyway: %+v", f.calls)
	}
}

// -v lists more units but must not change the verdict (exit status).
func TestFailingCountIgnoresVerboseOnlyUnits(t *testing.T) {
	h := &Host{Units: []*Unit{
		{Name: "ref.service", Kind: "service", Load: "not-found"},
		{Name: "vendor.service", Kind: "service", Load: "loaded", FileState: "static", Active: "inactive", Exec: []ExecCmd{{Key: "ExecStart", Path: "/sbin/quotaon"}}},
		{Name: "app.service", Kind: "service", Load: "loaded", FileState: "enabled", Active: "failed", Result: "exit-code"},
	}}
	facts := map[string]*UnitFacts{"vendor.service": {Facts: []Fact{{Req: probeReq{Kind: 'x', Role: "ExecStart", Path: "/sbin/quotaon", User: "root"}, State: "missing"}}}}
	if n := failingCount(h, facts); n != 1 {
		t.Errorf("got %d failing, want 1", n)
	}
}
