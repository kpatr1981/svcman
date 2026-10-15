// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
)

// detailLines describes u: state, files, commands, timer info, issues with
// numbered fixes. Returns the fixes in number order.
func (a *app) detailLines(u *Unit, w int) ([]string, []Fix) {
	var ls []string
	kv := func(k, v string) {
		if v != "" {
			ls = append(ls, wrapPrefixed(fmt.Sprintf("  %-10s ", k), strings.Repeat(" ", 13), v, w)...)
		}
	}
	title := bold(u.Name)
	if u.Description != "" {
		title += " — " + u.Description
	}
	ls = append(ls, title, "")
	state := u.Active + " (" + u.Sub + ")"
	if u.Active == "failed" {
		state = red("failed (" + u.Result + ")")
	}
	if !u.StateChange.IsZero() {
		state += dim("  since " + ago(a.h.clock(), u.StateChange) + " ago")
	}
	kv("State", state)
	kv("Enabled", u.FileState)
	kv("File", u.Fragment)
	for _, d := range u.DropIns {
		kv("Drop-in", d)
	}
	for _, e := range u.Exec {
		kv(e.Key, e.Argv)
	}
	kv("User", u.User)
	kv("Group", u.Group)
	kv("WorkDir", u.WorkDir)
	for _, e := range u.EnvFiles {
		kv("EnvFile", e.Path)
	}
	if u.Kind == "service" {
		kv("Restart", u.Restart+fmt.Sprintf("  (restarted %d times)", u.NRestarts))
	}
	if u.Kind == "timer" {
		kv("Triggers", strings.Join(u.Triggers, " "))
		if !u.NextElapse.IsZero() {
			kv("Next", u.NextElapse.Format("2006-01-02 15:04 UTC"))
		}
		if !u.LastTrigger.IsZero() {
			kv("Last", u.LastTrigger.Format("2006-01-02 15:04 UTC"))
		}
	}
	if len(u.TriggeredBy) > 0 {
		kv("By", strings.Join(u.TriggeredBy, " "))
	}
	ls = append(ls, "")
	var fixes []Fix
	is := a.issues(u)
	if _, probed := a.facts[u.Name]; !probed {
		ls = append(ls, dim("  files not checked yet"))
	}
	if len(is) == 0 {
		ls = append(ls, "  "+sevMark(SevOK)+" no problems found")
	}
	for _, i := range is {
		ls = append(ls, wrapPrefixed("  "+sevMark(i.Sev)+" ", "    ", i.Text, w)...)
		for _, f := range i.Fixes {
			fixes = append(fixes, f)
			ls = append(ls, "      "+cyan(fmt.Sprintf("[%d]", len(fixes)))+" "+f.Cmd+"  "+dim(f.Label))
		}
	}
	if u.Active == "failed" {
		if jl, err := a.h.journal(u.Name, 10); err == nil && len(jl) > 0 {
			ls = append(ls, "", dim("  last log lines:"))
			for _, l := range jl {
				ls = append(ls, "  "+l)
			}
		}
	}
	return ls, fixes
}

var detailKeys = [][2]string{
	{"1-9", "apply fix"}, {"s", "start"}, {"S", "stop"}, {"R", "restart"}, {"l", "journal"},
	{"e", "edit drop-in"}, {"c", "cat"},
}

func (a *app) details() {
	u := a.current()
	if _, ok := a.facts[u.Name]; !ok {
		a.status = "Checking " + u.Name + "…"
		a.scr.draw(a.render(nil), -1, -1)
	}
	a.recheck(u.Name)
	for {
		u = a.current()
		if u == nil {
			return
		}
		w, _ := a.scr.size()
		lines, fixes := a.detailLines(u, w)
		reopen := false
		a.pager("Details", lines, detailKeys, false, func(k Key) bool {
			switch {
			case k.Rune >= '1' && k.Rune <= '9':
				n := int(k.Rune - '0')
				if n <= len(fixes) {
					a.applyFix(fixes[n-1])
					reopen = true
					return true
				}
			case k.Rune == 's' || k.Rune == 'S' || k.Rune == 'R':
				a.doAction(a.render, map[rune]string{'s': "start", 'S': "stop", 'R': "restart"}[k.Rune])
				reopen = true
				return true
			case k.Rune == 'l':
				a.showJournal(u, 200)
			case k.Rune == 'e':
				a.editDropIn(u)
				reopen = true
				return true
			case k.Rune == 'c':
				a.showCat(u)
			}
			return false
		})
		if !reopen {
			return
		}
	}
}

func (a *app) applyFix(f Fix) {
	if k := a.ask(a.render, "Run as root: "+fit(f.Cmd, 200), "y = run   any other key = no"); k != 'y' && k != 'Y' {
		a.status = "Not applied."
		return
	}
	if !a.ensureRoot() {
		return
	}
	u := a.current()
	out, errOut, err := a.h.runRoot(f.Cmd + " 2>&1")
	if err != nil {
		a.status = red("fix failed: " + oneLine(out+" "+errOut+" "+err.Error()))
	} else {
		a.status = green("Applied: " + f.Label)
	}
	if u != nil {
		_ = a.h.refresh(append([]string{u.Name}, u.Triggers...)...)
		a.recheck(u.Name)
		a.rebuild()
	}
}

func (a *app) showJournal(u *Unit, n int) {
	ls, err := a.h.journal(u.Name, n)
	if err != nil {
		a.status = red("journal: " + err.Error())
		return
	}
	if len(ls) == 0 {
		ls = []string{dim("(no log entries)")}
	}
	if !a.h.journalRoot && !a.h.canRoot() && a.h.Priv != PrivRoot {
		a.status = dim("Showing what " + a.h.Login + " may read; system messages may be hidden.")
	}
	a.pager("Journal "+u.Name, ls, [][2]string{{"F", "follow live"}}, true, func(k Key) bool {
		if k.Rune == 'F' {
			a.follow(u)
		}
		return false
	})
}

// follow streams the journal in the real terminal until ctrl-c.
func (a *app) follow(u *Unit) {
	a.scr.suspend()
	fmt.Printf("\n%s %s on %s %s\n\n", bold("Following"), u.Name, a.h.r.Label(), dim("(ctrl-c to return)"))
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt) // ctrl-c ends journalctl, not svcman
	_ = a.h.r.Interactive(a.h.followCmd(u.Name))
	signal.Stop(sig)
	a.scr.resume()
}

func (a *app) showCat(u *Unit) {
	out, errOut, err := RunScript(a.h.r, "", envC+" systemctl cat --no-pager "+shq(u.Name)+" 2>&1")
	if err != nil && out == "" {
		a.status = red("cat: " + oneLine(errOut+" "+err.Error()))
		return
	}
	a.pager("systemctl cat "+u.Name, strings.Split(strings.TrimRight(out, "\n"), "\n"), nil, false, nil)
}

func (a *app) help() {
	ls := []string{
		bold("svcman "+version) + " — manage systemd services and timers",
		"",
		"   " + key("↑↓ j k") + "          move          " + key("/") + " search unit names   " + key("f") + " filter: all, failed, running, enabled, timers",
		"   " + key("s S R") + "           start / stop / restart (asks first; runs at once)",
		"   " + key("space t") + "         enable / disable (asks first; runs at once)",
		"   " + key("l") + "               journal (last 200 lines); " + key("F") + " follows it live, ctrl-c returns",
		"   " + key("enter") + "           details: files, commands, problems and numbered fixes (" + key("1-9") + " applies one)",
		"   " + key("e") + "               edit svcman's drop-in for the unit (/etc/systemd/system/<unit>.d/svcman.conf)",
		"   " + key("w") + "               save drop-in edits: diff, backup to ~/.svcman_backups/, daemon-reload",
		"   " + key("c") + "               systemctl cat (the unit as systemd sees it, drop-ins included)",
		"   " + key("r") + "               reload everything from the host",
		"",
		"CHECK column:  " + green("ok") + " no problems   " + red("✗ N") + " failing checks   " + yellow("! N") + " warnings   " + dim("·") + " not checked yet (open details)",
		"Timers show their service indented under them; SINCE shows the next run of a timer and the last result of its service.",
		"",
		"Root: actions use root directly, sudo -n, or ask your sudo password once (typed here, never shown).",
		"Vendor unit files and other drop-ins are never changed; delete svcman.conf to undo svcman's edits.",
		"",
		dim(author + "  " + repoURL),
	}
	a.pager("Help", ls, nil, false, nil)
}
