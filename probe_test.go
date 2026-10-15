// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequests(t *testing.T) {
	u := &Unit{Name: "a.service", Kind: "service", User: "app", Group: "app",
		Fragment: "/etc/systemd/system/a.service", DropIns: []string{"/etc/systemd/system/a.service.d/x.conf"},
		Exec:    []ExecCmd{{Key: "ExecStart", Path: "/opt/a"}, {Key: "ExecStartPre", Path: "/bin/true", IgnoreErrors: true}},
		WorkDir: "-/srv/a", EnvFiles: []EnvFile{{Path: "/etc/a.env", Optional: true}}}
	rs := requests(u)
	want := []string{"x ExecStart /opt/a app false", "x ExecStartPre /bin/true app true", "d WorkingDirectory /srv/a app true",
		"r EnvironmentFile /etc/a.env app true", "f unit file /etc/systemd/system/a.service root false",
		"f drop-in /etc/systemd/system/a.service.d/x.conf root false", "u User app  false", "g Group app  false",
		"s vendor a.service root false", "v verify /etc/systemd/system/a.service root false"}
	var got []string
	for _, r := range rs {
		got = append(got, strings.Join([]string{string(r.Kind), r.Role, r.Path, r.User, map[bool]string{true: "true", false: "false"}[r.Optional]}, " "))
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	// Default user is root; DynamicUser is checked as root and skips the user test; "~" WorkingDirectory is skipped.
	u2 := &Unit{Name: "b.service", Kind: "service", DynamicUser: true, User: "dyn", WorkDir: "~", Exec: []ExecCmd{{Key: "ExecStart", Path: "/b"}}}
	for _, r := range requests(u2) {
		if r.Kind == 'u' || r.Kind == 'd' || r.User != "root" && r.Kind == 'x' {
			t.Errorf("unexpected %+v", r)
		}
	}
}

func TestProbeScriptRunsLocally(t *testing.T) {
	if _, err := exec.LookPath("stat"); err != nil {
		t.Skip("no stat")
	}
	dir := t.TempDir()
	good := filepath.Join(dir, "good.sh")
	crlf := filepath.Join(dir, "crlf.sh")
	noexec := filepath.Join(dir, "noexec.sh")
	os.WriteFile(good, []byte("#!/bin/sh\necho\n"), 0o755)
	os.WriteFile(crlf, []byte("#!/bin/sh\r\necho\r\n"), 0o755)
	os.WriteFile(noexec, []byte("#!/bin/sh\necho\n"), 0o644)
	me := os.Getenv("USER")
	reqs := []probeReq{
		{Unit: "a", Role: "ExecStart", Kind: 'x', Path: good, User: me},
		{Unit: "a", Role: "ExecStart", Kind: 'x', Path: crlf, User: me},
		{Unit: "a", Role: "ExecStart", Kind: 'x', Path: noexec, User: me},
		{Unit: "a", Role: "ExecStart", Kind: 'x', Path: filepath.Join(dir, "nope"), User: me},
		{Unit: "a", Role: "User", Kind: 'u', Path: "nosuchuser_svcman"},
	}
	out, err := exec.Command("sh", "-c", probeScript(reqs)).CombinedOutput()
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	uf := parseProbe(string(out), reqs)["a"]
	if uf == nil || len(uf.Facts) != 5 {
		t.Fatalf("facts: %+v\n%s", uf, out)
	}
	f := uf.Facts
	if !f[0].Exists || f[0].Access != "ok" || f[0].Interp != "/bin/sh" || !f[0].InterpOK || f[0].CRLF || f[0].Mode != 0o755 {
		t.Errorf("good: %+v", f[0])
	}
	if !f[1].CRLF {
		t.Errorf("crlf: %+v", f[1])
	}
	if f[2].Access != "denied" {
		t.Errorf("noexec: %+v", f[2])
	}
	if f[3].Exists || f[3].State != "missing" {
		t.Errorf("missing: %+v", f[3])
	}
	if f[4].Exists {
		t.Errorf("user: %+v", f[4])
	}
}

func TestParseProbeVerify(t *testing.T) {
	reqs := []probeReq{{Unit: "a", Kind: 'v', Path: "/x"}}
	m := parseProbe("V\t0\t/x:3: Unknown key name 'Foo' in section 'Service', ignoring.\n", reqs)
	if len(m["a"].Verify) != 1 {
		t.Errorf("%+v", m["a"])
	}
}
