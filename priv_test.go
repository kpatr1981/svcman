// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"errors"
	"strings"
	"testing"
)

func TestRunRootModes(t *testing.T) {
	cases := []struct {
		priv    Priv
		pw      string
		wantCmd string
		wantIn  string
		wantErr error
	}{
		{PrivRoot, "", "/bin/sh -s", "echo hi", nil},
		{PrivNoPass, "", "sudo -n /bin/sh -s", "echo hi", nil},
		{PrivPassword, "s3cret", "sudo -k -S -p '' /bin/sh -s", "s3cret\necho hi", nil},
	}
	for _, c := range cases {
		f := &fakeRunner{}
		h := &Host{r: f, Priv: c.priv, pw: c.pw}
		_, _, err := h.runRoot("echo hi")
		if err != c.wantErr || len(f.calls) != 1 || f.calls[0].Cmd != c.wantCmd || f.calls[0].Stdin != c.wantIn {
			t.Errorf("priv %v: err=%v calls=%+v", c.priv, err, f.calls)
		}
	}
}

func TestRunRootNeedsPassword(t *testing.T) {
	f := &fakeRunner{}
	h := &Host{r: f, Priv: PrivPassword}
	if _, _, err := h.runRoot("true"); !errors.Is(err, errNeedPassword) || len(f.calls) != 0 {
		t.Errorf("want errNeedPassword and no call, got %v %+v", err, f.calls)
	}
	h.Priv = PrivNone
	if _, _, err := h.runRoot("true"); !errors.Is(err, errNoRoot) {
		t.Errorf("want errNoRoot, got %v", err)
	}
}

// A remote sudo prompt would hang a non-tty ssh session: every sudo call must be -n or -S.
func TestRunRootNeverPromptsRemotely(t *testing.T) {
	for _, p := range []Priv{PrivRoot, PrivNoPass, PrivPassword} {
		f := &fakeRunner{}
		h := &Host{r: f, Priv: p, pw: "x"}
		h.runRoot("true")
		h.action("restart", "a.service")
		for _, c := range f.calls {
			if strings.Contains(c.Cmd, "sudo") && !strings.Contains(c.Cmd, "sudo -n") && !strings.Contains(c.Cmd, "sudo -k -S") {
				t.Errorf("priv %v: prompting sudo call %q", p, c.Cmd)
			}
		}
	}
}

func TestSetPassword(t *testing.T) {
	f := &fakeRunner{reply: func(cmd, in string) (string, string, error) {
		if in == "good\n" {
			return "SUDO_OK\n", "", nil
		}
		return "", "Sorry, try again.", errors.New("exit 1")
	}}
	h := &Host{r: f, Priv: PrivPassword}
	if err := h.setPassword("bad"); err == nil || h.pw != "" {
		t.Errorf("bad password accepted")
	}
	if err := h.setPassword("good"); err != nil || h.pw != "good" || !h.canRoot() {
		t.Errorf("good password rejected: %v", err)
	}
}

func TestActionCommand(t *testing.T) {
	f := &fakeRunner{reply: func(cmd, in string) (string, string, error) {
		return "Job for x.service failed.\nSee journalctl.\n", "", errors.New("exit 1")
	}}
	h := &Host{r: f, Priv: PrivRoot}
	_, err := h.action("restart", "x.service", "y.timer")
	if f.calls[0].Stdin != envC+" systemctl restart x.service y.timer 2>&1" {
		t.Errorf("script %q", f.calls[0].Stdin)
	}
	if err == nil || err.Error() != "Job for x.service failed. See journalctl." {
		t.Errorf("err %v", err)
	}
	h.action("daemon-reload")
	if f.calls[1].Stdin != envC+" systemctl daemon-reload 2>&1" {
		t.Errorf("daemon-reload script %q", f.calls[1].Stdin)
	}
}

// Linux limits one argument to 128 KiB: a large probe script must travel on
// stdin (after the password line, which is all sudo -S reads), never in argv.
func TestRunRootLargeScriptViaStdin(t *testing.T) {
	f := &fakeRunner{}
	h := &Host{r: f, Priv: PrivPassword, pw: "pw"}
	big := strings.Repeat("x", 200000)
	h.runRoot(big)
	if len(f.calls[0].Cmd) > 1000 || !strings.HasSuffix(f.calls[0].Stdin, big) {
		t.Errorf("script not on stdin: cmd %d bytes", len(f.calls[0].Cmd))
	}
}

// Login shells on the host can print locale warnings on every command; they
// must not end up in svcman's error messages.
func TestOneLineDropsLoginNoise(t *testing.T) {
	in := "bash: warning: setlocale: LC_ALL: cannot change locale (en_US.UTF-8)\n" +
		"/bin/sh: warning: setlocale: LC_ALL: cannot change locale (en_US.UTF-8)\nJob for a.service failed.\n"
	if got := oneLine(in); got != "Job for a.service failed." {
		t.Errorf("%q", got)
	}
}

func TestLoadHostErrorDropsLoginNoise(t *testing.T) {
	f := &fakeRunner{reply: func(cmd, in string) (string, string, error) {
		return "", "bash: warning: setlocale: LC_ALL: cannot change locale (en_US.UTF-8)\nPermission denied\n", errors.New("exit 1")
	}}
	_, err := loadHost(f)
	if err == nil || strings.Contains(err.Error(), "setlocale") || !strings.Contains(err.Error(), "Permission denied") {
		t.Errorf("%v", err)
	}
}
