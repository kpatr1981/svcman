// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"fmt"
	"strings"
	"time"
)

// Check returns the problems found from u's properties and, when f is not
// nil, from the probe's facts about its files and users.
func Check(h *Host, u *Unit, f *UnitFacts) []Issue {
	var is []Issue
	add := func(s Severity, text string, fixes ...Fix) { is = append(is, Issue{s, text, fixes}) }
	q := shq(u.Name)

	switch u.Load {
	case "not-found", "bad-setting", "error":
		add(SevFail, "unit does not load (LoadState="+u.Load+")")
		return is
	case "masked":
		add(SevInfo, "masked (cannot be started)")
		return is
	}
	if u.Active == "failed" {
		add(SevFail, fmt.Sprintf("failed (%s)", u.Result), failFix(u))
	}
	if u.NRestarts >= 5 {
		add(SevWarn, fmt.Sprintf("restarted %d times since it was started (restart loop?)", u.NRestarts))
	}
	// Only a unit that ran and was stopped: one that never started (start
	// condition not met, its target never reached) is normal.
	if u.Kind == "service" && u.FileState == "enabled" && u.Active == "inactive" &&
		len(u.TriggeredBy) == 0 && u.Type != "oneshot" && !u.LastStart.IsZero() {
		add(SevWarn, "enabled but not running", Fix{"start it", "systemctl start " + q})
	}
	if u.NeedReload {
		add(SevWarn, "unit file changed on disk; systemd needs a daemon-reload", Fix{"reload systemd", "systemctl daemon-reload"})
	}
	if u.Kind == "timer" {
		for _, t := range u.Triggers {
			tu := h.Unit(t)
			if tu == nil || tu.Load == "not-found" {
				add(SevFail, "triggers "+t+", which does not exist")
				continue
			}
			if tu.Result != "" && tu.Result != "success" {
				add(SevWarn, fmt.Sprintf("last run of %s failed (%s)", t, tu.Result))
			}
		}
		if u.Active == "active" && !u.NextElapse.IsZero() && h.clock().Sub(u.NextElapse) > 5*time.Minute {
			add(SevWarn, "overdue: it should have triggered at "+u.NextElapse.Format("2006-01-02 15:04")+" UTC",
				Fix{"restart the timer", "systemctl restart " + q})
		}
	}
	if f != nil {
		is = append(is, checkFacts(u, f)...)
	}
	return is
}

// failFix clears a failed unit and starts it again.
func failFix(u *Unit) Fix {
	q := shq(u.Name)
	return Fix{"reset the failure and restart", "systemctl reset-failed " + q + " && systemctl restart " + q}
}

// checkFacts turns probe facts into issues.
func checkFacts(u *Unit, uf *UnitFacts) []Issue {
	var is []Issue
	add := func(s Severity, text string, fixes ...*Fix) {
		var fs []Fix
		for _, f := range fixes {
			if f != nil {
				fs = append(fs, *f)
			}
		}
		is = append(is, Issue{s, text, fs})
	}
	// A user that does not exist is reported once; its access results are noise.
	badUser := map[string]bool{}
	for _, f := range uf.Facts {
		if f.Req.Kind == 'u' && !f.Exists {
			badUser[f.Req.Path] = true
		}
	}
	var user string
	denied := func(a string) bool { return !badUser[user] && (a == "denied" || a == "approx-denied") }
	approx := func(a string) string {
		if strings.HasPrefix(a, "approx") {
			return " (judged from permission bits)"
		}
		return ""
	}
	for _, f := range uf.Facts {
		r := f.Req
		user = r.User
		missing := func() {
			what := "does not exist"
			if f.State == "broken-link" {
				what = "is a broken symlink"
			} else if r.Kind == 'x' && !strings.HasPrefix(r.Path, "/") {
				what = "is not in systemd's search path"
			}
			sev := SevFail
			if r.Optional {
				sev = SevWarn
			}
			add(sev, fmt.Sprintf("%s %s %s", r.Role, r.Path, what))
		}
		switch r.Kind {
		case 'x':
			if !f.Exists {
				missing()
				continue
			}
			if f.Type != "file" {
				add(SevFail, fmt.Sprintf("%s %s is not a regular file", r.Role, r.Path))
				continue
			}
			if denied(f.Access) {
				add(SevFail, fmt.Sprintf("%s is not executable by %s%s", r.Path, r.User, approx(f.Access)), chmodFix(f, "x"))
			}
			if f.Interp != "" && !f.InterpOK {
				add(SevFail, fmt.Sprintf("%s: interpreter %s does not exist", r.Path, f.Interp))
			}
			if f.EnvCmd != "" && !f.EnvCmdOK {
				add(SevFail, fmt.Sprintf("%s: env cannot find %s in systemd's PATH", r.Path, f.EnvCmd))
			}
			if f.Interp != "" && denied(f.AccessR) {
				add(SevFail, fmt.Sprintf("%s is a script but not readable by %s%s", r.Path, r.User, approx(f.AccessR)), chmodFix(f, "r"))
			}
			if f.CRLF {
				add(SevFail, r.Path+" has Windows (CRLF) line endings", &Fix{"strip the CR characters", `sed -i 's/\r$//' ` + shq(r.Path)})
			}
			if f.NoExec {
				add(SevFail, r.Path+" is on a filesystem mounted noexec")
			}
		case 'd':
			if !f.Exists {
				if !r.Optional {
					missing()
				}
				continue
			}
			if f.Type != "dir" {
				add(SevFail, fmt.Sprintf("WorkingDirectory %s is not a directory", r.Path))
			} else if denied(f.Access) {
				add(SevFail, fmt.Sprintf("%s cannot enter WorkingDirectory %s%s", r.User, r.Path, approx(f.Access)), chmodFix(f, "x"))
			}
		case 'r':
			if !f.Exists {
				if !r.Optional {
					missing()
				}
				continue
			}
			if denied(f.Access) {
				add(SevFail, fmt.Sprintf("%s %s is not readable by %s%s", r.Role, r.Path, r.User, approx(f.Access)), chmodFix(f, "r"))
			}
		case 'f':
			if !f.Exists {
				continue
			}
			if f.Mode&0o022 != 0 {
				add(SevFail, fmt.Sprintf("%s %s is writable by group or others (%03o)", r.Role, r.Path, f.Mode),
					&Fix{"remove group/other write", "chmod go-w " + shq(r.Path)})
			}
			if f.Owner != "root" {
				add(SevFail, fmt.Sprintf("%s %s is owned by %s, not root", r.Role, r.Path, f.Owner),
					&Fix{"give it to root", "chown root:root " + shq(r.Path)})
			}
		case 'u', 'g':
			if !f.Exists {
				what := "user"
				if r.Kind == 'g' {
					what = "group"
				}
				add(SevFail, fmt.Sprintf("%s %s does not exist", what, r.Path))
			}
		case 's':
			if f.Exists {
				add(SevInfo, "overrides the vendor unit of the same name in /usr/lib (left by systemctl edit --full?)")
			}
		}
	}
	// verify loads every unit and also prints their warnings: keep this unit's.
	for _, v := range uf.Verify {
		if (u.Fragment != "" && strings.Contains(v, u.Fragment)) || strings.HasPrefix(v, u.Name+":") {
			add(SevWarn, "systemd-analyze: "+v)
		}
	}
	return is
}

// chmodFix adds the missing permission bit for the class the unit's user
// falls in (owner, else others; group membership is not probed). It returns
// nil when that bit is already set, because then something else (a parent
// directory, ACL, mount) blocks access and chmod on the file would not help.
func chmodFix(f Fact, bit string) *Fix {
	mask := map[string]int{"r": 4, "x": 1}[bit]
	who, shift := "o", 0
	if f.Req.User == "root" || f.Req.User == f.Owner {
		who, shift = "u", 6
	}
	if f.Mode&(mask<<shift) != 0 {
		return nil
	}
	return &Fix{"chmod " + who + "+" + bit, "chmod " + who + "+" + bit + " " + shq(f.Req.Path)}
}
