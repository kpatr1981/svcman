// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderResetsExecKeys(t *testing.T) {
	d := newDropIn("a.service", "", false)
	d.Entries = []Entry{
		{"Service", "ExecStart", "/opt/new --x"},
		{"Service", "Environment", "A=1"},
		{"Unit", "Description", "x"},
		{"Service", "ExecStart", "/opt/second"},
	}
	want := dropInHeader + "[Service]\nExecStart=\nExecStart=/opt/new --x\nEnvironment=A=1\nExecStart=/opt/second\n\n[Unit]\nDescription=x\n"
	if got := d.Render(); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestParseRoundTrip(t *testing.T) {
	src := "# comment\n[Service]\nExecStart=\nExecStart=/opt/x\nRestart=always\n\n[Unit]\nDescription=y\n"
	d := newDropIn("a.service", src, true)
	if len(d.Entries) != 3 || d.Entries[0] != (Entry{"Service", "ExecStart", "/opt/x"}) {
		t.Fatalf("%+v", d.Entries)
	}
	if d.Dirty() {
		t.Error("unchanged drop-in reported dirty")
	}
	d.Entries = d.Entries[:2]
	if !d.Dirty() {
		t.Error("edit not detected")
	}
}

func TestEmptyRendersEmpty(t *testing.T) {
	d := newDropIn("a.service", "[Service]\nRestart=always\n", true)
	d.Entries = nil
	if d.Render() != "" || !d.Dirty() {
		t.Error("removing every key must render empty and be dirty")
	}
}

func TestKnownKey(t *testing.T) {
	props := map[string]bool{"Restart": true, "RestartUSec": true, "EnvironmentFiles": true, "TimeoutStartUSec": true}
	for _, k := range []string{"Restart", "RestartSec", "EnvironmentFile", "TimeoutStartSec"} {
		if !knownKey(props, k) {
			t.Errorf("%s should be known", k)
		}
	}
	if knownKey(props, "Restrat") {
		t.Error("typo accepted")
	}
}

// saveScript is run locally with a fake systemctl to test the guard,
// backup, write and rollback logic.
func runSave(t *testing.T, root, orig, onDisk string, exists bool, content string, reloadRC string) (string, error) {
	t.Helper()
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\necho \"systemctl $*\" >> "+root+"/calls\nexit "+reloadRC+"\n"), 0o755)
	dir := filepath.Join(root, "units")
	ddir := filepath.Join(dir, "a.service.d")
	if onDisk != "" {
		os.MkdirAll(ddir, 0o755)
		os.WriteFile(filepath.Join(ddir, "svcman.conf"), []byte(onDisk), 0o644)
	}
	d := newDropIn("a.service", orig, exists)
	d.Entries = parseDropIn(content)
	cmd := exec.Command("sh", "-c", saveScript(dir, d, d.Render(), filepath.Join(root, "bk"), os.Getenv("USER"), "20261015-0400"))
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestSaveScriptWritesAndBacksUp(t *testing.T) {
	root := t.TempDir()
	old := "[Service]\nRestart=no\n"
	out, err := runSave(t, root, old, old, true, "[Service]\nRestart=always\n", "0")
	if err != nil || !strings.Contains(out, "OK") || !strings.Contains(out, "BACKUP ") {
		t.Fatalf("%v %s", err, out)
	}
	b, _ := os.ReadFile(filepath.Join(root, "units/a.service.d/svcman.conf"))
	if !strings.Contains(string(b), "Restart=always") {
		t.Errorf("not written: %s", b)
	}
	bk, _ := os.ReadFile(filepath.Join(root, "bk/a.service-20261015-0400.conf"))
	if string(bk) != old {
		t.Errorf("backup %q", bk)
	}
	calls, _ := os.ReadFile(filepath.Join(root, "calls"))
	if strings.TrimSpace(string(calls)) != "systemctl daemon-reload" {
		t.Errorf("calls %q", calls)
	}
}

func TestSaveScriptRefusesChangedFile(t *testing.T) {
	root := t.TempDir()
	out, _ := runSave(t, root, "[Service]\nRestart=no\n", "[Service]\nRestart=on-failure\n", true, "[Service]\nRestart=always\n", "0")
	if !strings.Contains(out, "CHANGED") {
		t.Fatalf("want CHANGED, got %s", out)
	}
	b, _ := os.ReadFile(filepath.Join(root, "units/a.service.d/svcman.conf"))
	if string(b) != "[Service]\nRestart=on-failure\n" {
		t.Errorf("file touched: %q", b)
	}
	// A file that appeared after svcman loaded "no file" is also a change.
	root2 := t.TempDir()
	if out, _ := runSave(t, root2, "", "[Service]\nX=1\n", false, "[Service]\nRestart=always\n", "0"); !strings.Contains(out, "CHANGED") {
		t.Errorf("new file not detected: %s", out)
	}
}

func TestSaveScriptToleratesMissingFinalNewline(t *testing.T) {
	root := t.TempDir()
	out, err := runSave(t, root, "[Service]\nRestart=no", "[Service]\nRestart=no", true, "[Service]\nRestart=always\n", "0")
	if err != nil || !strings.Contains(out, "OK") {
		t.Errorf("%v %s", err, out)
	}
}

func TestSaveScriptRollsBackOnReloadFailure(t *testing.T) {
	root := t.TempDir()
	old := "[Service]\nRestart=no\n"
	out, _ := runSave(t, root, old, old, true, "[Service]\nRestart=always\n", "1")
	if !strings.Contains(out, "RELOADFAIL") {
		t.Fatalf("want RELOADFAIL, got %s", out)
	}
	b, _ := os.ReadFile(filepath.Join(root, "units/a.service.d/svcman.conf"))
	if string(b) != old {
		t.Errorf("not restored: %q", b)
	}
	// New file + failed reload: the new file is removed.
	root2 := t.TempDir()
	runSave(t, root2, "", "", false, "[Service]\nRestart=always\n", "1")
	if _, err := os.Stat(filepath.Join(root2, "units/a.service.d/svcman.conf")); !os.IsNotExist(err) {
		t.Error("new file left behind after failed reload")
	}
}

func TestSaveScriptDeletesEmpty(t *testing.T) {
	root := t.TempDir()
	old := "[Service]\nRestart=no\n"
	out, err := runSave(t, root, old, old, true, "", "0")
	if err != nil || !strings.Contains(out, "OK") {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, "units/a.service.d")); !os.IsNotExist(err) {
		t.Error("empty drop-in dir not removed")
	}
}

func TestSaveDropInMapsResults(t *testing.T) {
	for out, want := range map[string]string{
		"BACKUP /h/.svcman_backups/a.service-x.conf\nOK\n": "",
		"CHANGED\n":    errChanged.Error(),
		"RELOADFAIL\n": "daemon-reload failed",
	} {
		f := &fakeRunner{reply: func(cmd, in string) (string, string, error) { return out, "", nil }}
		h := &Host{r: f, Priv: PrivRoot, Home: "/h", Login: "alice"}
		d := newDropIn("a.service", "", false)
		_, err := saveDropIn(h, d, "x")
		if (want == "" && err != nil) || (want != "" && (err == nil || !strings.Contains(err.Error(), want))) {
			t.Errorf("%q: got %v", out, err)
		}
	}
}

// After a save the written content is the new baseline: later edits in the
// same session are changes against it, and the guard expects it on disk.
func TestMarkSaved(t *testing.T) {
	d := newDropIn("a.service", "", false)
	d.Entries = []Entry{{"Service", "Restart", "always"}}
	d.markSaved()
	if d.Dirty() || !d.Exists || d.Orig != d.Render() {
		t.Fatalf("not clean after save: %+v", d)
	}
	d.Entries = nil
	if !d.Dirty() {
		t.Error("deleting the saved entry must be a change")
	}
	d.markSaved()
	if d.Exists || d.Orig != "" {
		t.Errorf("saving an empty drop-in deletes the file: %+v", d)
	}
}

// The unit name is quoted wherever it reaches the shell, backup path included.
func TestSaveScriptQuotesUnitInBackupPath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "units")
	unit := `a$HOME"x.service`
	os.MkdirAll(filepath.Join(dir, unit+".d"), 0o755)
	old := "[Service]\nRestart=no\n"
	os.WriteFile(filepath.Join(dir, unit+".d", "svcman.conf"), []byte(old), 0o644)
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0o755)
	os.WriteFile(filepath.Join(bin, "systemctl"), []byte("#!/bin/sh\nexit 0\n"), 0o755)
	d := newDropIn(unit, old, true)
	d.Entries = parseDropIn("[Service]\nRestart=always\n")
	cmd := exec.Command("sh", "-c", saveScript(dir, d, d.Render(), filepath.Join(root, "bk"), os.Getenv("USER"), "s"))
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "OK") {
		t.Fatalf("%v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(root, "bk", unit+"-s.conf")); err != nil {
		t.Errorf("backup not at the literal name: %v\n%s", err, out)
	}
}
