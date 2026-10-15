// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseShowBlocks(t *testing.T) {
	out := `Id=a.service
Type=simple
LoadState=loaded
ActiveState=failed
SubState=failed
Result=exit-code
UnitFileState=enabled
FragmentPath=/etc/systemd/system/a.service
DropInPaths=/etc/systemd/system/a.service.d/x.conf /etc/systemd/system/a.service.d/svcman.conf
ExecStart={ path=/opt/a/run ; argv[]=/opt/a/run --x ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 }
User=app
EnvironmentFiles=/etc/default/a (ignore_errors=yes)
EnvironmentFiles=/etc/a.env (ignore_errors=no)
NRestarts=7
TriggeredBy=a.timer
NeedDaemonReload=yes
StateChangeTimestamp=Thu 2026-10-15 02:00:00 UTC

Id=a.timer
LoadState=loaded
ActiveState=active
Triggers=a.service
NextElapseUSecRealtime=Fri 2026-10-16 03:00:00 UTC
LastTriggerUSec=n/a
`
	us := parseShow(out)
	if len(us) != 2 {
		t.Fatalf("got %d units", len(us))
	}
	a := us[0]
	if a.Name != "a.service" || a.Kind != "service" || a.Active != "failed" || a.Result != "exit-code" || a.FileState != "enabled" {
		t.Errorf("bad states: %+v", a)
	}
	if len(a.DropIns) != 2 || a.DropIns[1] != "/etc/systemd/system/a.service.d/svcman.conf" {
		t.Errorf("dropins %v", a.DropIns)
	}
	if len(a.Exec) != 1 || a.Exec[0].Path != "/opt/a/run" || a.Exec[0].Argv != "/opt/a/run --x" || a.Exec[0].Key != "ExecStart" {
		t.Errorf("exec %+v", a.Exec)
	}
	if len(a.EnvFiles) != 2 || !a.EnvFiles[0].Optional || a.EnvFiles[1].Optional || a.EnvFiles[1].Path != "/etc/a.env" {
		t.Errorf("envfiles %+v", a.EnvFiles)
	}
	if a.NRestarts != 7 || !a.NeedReload || a.User != "app" || len(a.TriggeredBy) != 1 {
		t.Errorf("misc %+v", a)
	}
	if !a.StateChange.Equal(time.Date(2026, 10, 15, 2, 0, 0, 0, time.UTC)) {
		t.Errorf("time %v", a.StateChange)
	}
	tm := us[1]
	if tm.Kind != "timer" || tm.Triggers[0] != "a.service" || !tm.LastTrigger.IsZero() || tm.NextElapse.Day() != 16 {
		t.Errorf("timer %+v", tm)
	}
}

func TestParseExecMulti(t *testing.T) {
	v := `{ path=/bin/mkdir ; argv[]=/bin/mkdir -p /run/x ; ignore_errors=yes ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 } ; { path=/usr/bin/sh ; argv[]=/usr/bin/sh -c echo a ; b ; ignore_errors=no ; start_time=[n/a] ; stop_time=[n/a] ; pid=0 ; code=(null) ; status=0/0 }`
	es := parseExec("ExecStartPre", v)
	if len(es) != 2 {
		t.Fatalf("got %d: %+v", len(es), es)
	}
	if es[0].Path != "/bin/mkdir" || !es[0].IgnoreErrors || es[1].Path != "/usr/bin/sh" || es[1].Argv != "/usr/bin/sh -c echo a ; b" {
		t.Errorf("%+v", es)
	}
}

func TestParseTime(t *testing.T) {
	for _, s := range []string{"", "n/a", "garbage"} {
		if !parseTime(s).IsZero() {
			t.Errorf("%q should be zero", s)
		}
	}
}

func TestParseShowFixtures(t *testing.T) {
	for _, f := range []string{"testdata/show-debian.txt", "testdata/show-rocky.txt"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		us := parseShow(string(b))
		by := map[string]*Unit{}
		for _, u := range us {
			by[u.Name] = u
		}
		if len(us) < 15 {
			t.Errorf("%s: only %d units", f, len(us))
		}
		m := by["svct-missing.service"]
		if m == nil || len(m.Exec) == 0 || m.Exec[0].Path != "/opt/svct/nope" {
			t.Errorf("%s: svct-missing %+v", f, m)
		}
		if u := by["svct-loop.service"]; u == nil || u.NRestarts < 2 {
			t.Errorf("%s: svct-loop restarts %+v", f, u)
		}
		if u := by["svct-reload.service"]; u == nil || !u.NeedReload {
			t.Errorf("%s: svct-reload NeedReload not set", f)
		}
		if u := by["svct-orphan.timer"]; u == nil || len(u.Triggers) != 1 || u.Triggers[0] != "svct-ghost.service" {
			t.Errorf("%s: orphan timer %+v", f, u)
		}
		if u := by["svct-good.timer"]; u == nil || u.NextElapse.IsZero() {
			t.Errorf("%s: good timer has no next elapse", f)
		}
		inst := false
		for _, u := range us {
			inst = inst || strings.Contains(u.Name, "@") && u.Kind == "service"
		}
		if !inst {
			t.Errorf("%s: no template instance (foo@bar.service) parsed", f)
		}
	}
}

// Aliases (sshd.service -> ssh.service) make systemctl show print the same
// unit twice; the list must hold it once.
func TestParseShowDedupesAliases(t *testing.T) {
	us := parseShow("Id=ssh.service\nActiveState=active\n\nId=ssh.service\nActiveState=active\n\nId=b.service\n")
	if len(us) != 2 || us[0].Name != "ssh.service" || us[1].Name != "b.service" {
		t.Errorf("got %d units", len(us))
	}
}
