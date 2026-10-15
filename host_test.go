// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestParseLoad(t *testing.T) {
	out := "@@INFO\n1760490000\nalice\n/home/alice\npass\ndegraded\n@@SHOW\nId=a.service\nActiveState=active\n\nId=b.timer\nTriggers=a.service\n"
	h, err := parseLoad(out)
	if err != nil {
		t.Fatal(err)
	}
	if h.Login != "alice" || h.Home != "/home/alice" || h.Priv != PrivPassword || h.SysState != "degraded" || h.Now.Unix() != 1760490000 {
		t.Errorf("%+v", h)
	}
	if len(h.Units) != 2 || h.Unit("b.timer") == nil || h.Unit("nope") != nil {
		t.Errorf("units %+v", h.Units)
	}
}

func TestParseLoadNoSystemd(t *testing.T) {
	if _, err := parseLoad("@@NOSYSTEMD\n"); err == nil || !strings.Contains(err.Error(), "systemd") {
		t.Errorf("want no-systemd error, got %v", err)
	}
}

func TestParseLoadPriv(t *testing.T) {
	for word, want := range map[string]Priv{"root": PrivRoot, "nopass": PrivNoPass, "pass": PrivPassword, "none": PrivNone} {
		h, err := parseLoad("@@INFO\n1\nx\n/x\n" + word + "\nrunning\n@@SHOW\n")
		if err != nil || h.Priv != want {
			t.Errorf("%s: %v %v", word, h.Priv, err)
		}
	}
}

func TestLoadScriptIsValidSh(t *testing.T) {
	cmd := exec.Command("sh", "-n")
	cmd.Stdin = strings.NewReader(loadScript)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sh -n: %v %s", err, out)
	}
}

// Template files (foo@.service) cannot be shown; instances (foo@bar.service) can.
// Names like systemd-fsck@dev-disk-by\x2duuid-1.service must reach systemctl
// unchanged: plain xargs would eat the backslash.
func TestLoadScriptSkipsTemplates(t *testing.T) {
	if !strings.Contains(loadScript, `grep -v '@\.'`) {
		t.Error("load script must drop template unit files")
	}
	cmd := exec.Command("sh", "-c", `printf '%s\n' 'a@.service' 'b@x.service' 'c\x2dd.service' | grep -v '@\.' | tr '\n' '\0' | xargs -0 printf '[%s]'`)
	out, _ := cmd.Output()
	if string(out) != `[b@x.service][c\x2dd.service]` || !strings.Contains(loadScript, "xargs -0") {
		t.Errorf("name pipeline: %s", out)
	}
}

func TestRefreshReplacesUnits(t *testing.T) {
	b, _ := os.ReadFile("testdata/show-debian.txt")
	h := &Host{Units: parseShow(string(b))}
	f := &fakeRunner{reply: func(cmd, stdin string) (string, string, error) {
		return "Id=svct-loop.service\nActiveState=inactive\nNRestarts=0\n", "", nil
	}}
	h.r = f
	n := len(h.Units)
	if err := h.refresh("svct-loop.service"); err != nil {
		t.Fatal(err)
	}
	if len(h.Units) != n || h.Unit("svct-loop.service").Active != "inactive" {
		t.Errorf("refresh did not replace in place")
	}
	if !strings.Contains(f.calls[0].Stdin, "svct-loop.service") || !strings.Contains(f.calls[0].Stdin, "TZ=UTC") {
		t.Errorf("refresh script: %q", f.calls[0].Stdin)
	}
}

// Ages are measured against the host's clock now, not when it was loaded.
func TestHostClockAdvances(t *testing.T) {
	h, _ := parseLoad("@@INFO\n1760490000\nx\n/x\nroot\nrunning\n@@SHOW\n")
	h.loadedAt = h.loadedAt.Add(-time.Hour)
	if got := h.clock().Sub(h.Now); got < time.Hour || got > time.Hour+time.Minute {
		t.Errorf("clock advanced %v, want ~1h", got)
	}
	if (&Host{Now: h.Now}).clock() != h.Now {
		t.Error("a host built in a test (no loadedAt) uses Now")
	}
}
