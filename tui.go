// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

var filters = []string{"all", "failed", "running", "enabled", "timers"}

type row struct {
	u     *Unit
	child bool // a timer's service, shown under the timer
}

type app struct {
	h        *Host
	scr      *screen
	rows     []row
	sel, off int
	filter   int
	search   string
	status   string
	facts    map[string]*UnitFacts
	edits    map[string]*DropIn
}

// buildRows orders units (failed first, then by name), puts each timer's
// service under it, and applies the filter and search.
func buildRows(h *Host, filter int, search string) []row {
	underTimer := map[string]bool{}
	for _, u := range h.Units {
		if u.Kind == "timer" {
			for _, t := range u.Triggers {
				if h.Unit(t) != nil {
					underTimer[t] = true
				}
			}
		}
	}
	top := make([]*Unit, 0, len(h.Units))
	for _, u := range h.Units {
		if !underTimer[u.Name] && u.Load != "not-found" {
			top = append(top, u)
		}
	}
	sort.SliceStable(top, func(i, j int) bool {
		fi, fj := top[i].Active == "failed", top[j].Active == "failed"
		if fi != fj {
			return fi
		}
		return top[i].Name < top[j].Name
	})
	children := func(u *Unit) []*Unit {
		var cs []*Unit
		if u.Kind == "timer" {
			for _, t := range u.Triggers {
				if c := h.Unit(t); c != nil {
					cs = append(cs, c)
				}
			}
		}
		return cs
	}
	match := func(u *Unit) bool {
		cs := children(u)
		switch filters[filter] {
		case "failed":
			if u.Active == "failed" {
				return true
			}
			for _, c := range cs {
				if c.Active == "failed" || c.Result != "" && c.Result != "success" {
					return true
				}
			}
			return false
		case "running":
			return u.Sub == "running"
		case "enabled":
			return u.FileState == "enabled" || u.FileState == "enabled-runtime"
		case "timers":
			return u.Kind == "timer"
		}
		return true
	}
	var rows []row
	for _, u := range top {
		cs := children(u)
		if !match(u) {
			continue
		}
		if search != "" {
			hit := strings.Contains(u.Name, search)
			for _, c := range cs {
				hit = hit || strings.Contains(c.Name, search)
			}
			if !hit {
				continue
			}
		}
		rows = append(rows, row{u: u})
		for _, c := range cs {
			rows = append(rows, row{u: c, child: true})
		}
	}
	return rows
}

func (a *app) rebuild() {
	var cur string
	if u := a.current(); u != nil {
		cur = u.Name
	}
	a.rows = buildRows(a.h, a.filter, a.search)
	a.sel = 0
	for i, r := range a.rows {
		if r.u.Name == cur {
			a.sel = i
		}
	}
}

func (a *app) current() *Unit {
	if a.sel >= 0 && a.sel < len(a.rows) {
		return a.rows[a.sel].u
	}
	return nil
}

// issues are the unit's checks; property checks only until it is probed.
func (a *app) issues(u *Unit) []Issue { return Check(a.h, u, a.facts[u.Name]) }

func (a *app) checkCell(u *Unit) string {
	if _, ok := a.facts[u.Name]; !ok {
		return dim("·")
	}
	is := a.issues(u)
	n := func(s Severity) int {
		c := 0
		for _, i := range is {
			if i.Sev == s {
				c++
			}
		}
		return c
	}
	switch worst(is) {
	case SevFail:
		return red(fmt.Sprintf("✗ %d", n(SevFail)))
	case SevWarn:
		return yellow(fmt.Sprintf("! %d", n(SevWarn)))
	}
	return green("ok")
}

func ago(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := now.Sub(t)
	if d < 0 {
		d = -d
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func (a *app) sinceCell(r row) string {
	u, now := r.u, a.h.clock()
	if u.Kind == "timer" {
		if u.NextElapse.IsZero() {
			return "-"
		}
		return "next " + u.NextElapse.Format("01-02 15:04")
	}
	if r.child {
		mark := green("✓")
		if u.Result != "success" && u.Result != "" {
			mark = red("✗")
		}
		return "last " + mark + " " + ago(now, u.StateChange)
	}
	return ago(now, u.StateChange)
}

func activeCell(u *Unit) string {
	s := u.Sub
	if u.Kind == "timer" && u.Active == "active" {
		s = "waiting"
	}
	switch u.Active {
	case "failed":
		return red(s)
	case "active":
		return green(s)
	}
	return dim(s)
}

var mainKeys = [][2]string{
	{"↑↓", "move"}, {"/", "search"}, {"f", "filter"}, {"s", "start"}, {"S", "stop"}, {"R", "restart"},
	{"space", "enable/disable"}, {"l", "journal"}, {"enter", "details & fixes"}, {"e", "edit drop-in"},
	{"w", "save"}, {"c", "cat"}, {"r", "reload"}, {"?", "help"}, {"q", "quit"},
}

func (a *app) dirtyCount() int {
	n := 0
	for _, d := range a.edits {
		if d.Dirty() {
			n++
		}
	}
	return n
}

func (a *app) titleLine() string {
	svc, tim, failed := 0, 0, 0
	for _, u := range a.h.Units {
		if u.Kind == "timer" {
			tim++
		} else {
			svc++
		}
		if u.Active == "failed" {
			failed++
		}
	}
	t := inv(bold(" svcman ")) + " " + bold(a.h.Login+"@"+hostOf(a.h.r.Label())) +
		dim(fmt.Sprintf("  %d services, %d timers  ·  filter: %s", svc, tim, filters[a.filter]))
	if a.search != "" {
		t += dim("  ·  search: ") + a.search
	}
	if failed > 0 {
		t += "  " + red(fmt.Sprintf("%d failed", failed))
	}
	if a.h.SysState != "running" {
		t += "  " + yellow(a.h.SysState)
	}
	if n := a.dirtyCount(); n > 0 {
		t += "  " + yellow(fmt.Sprintf("● %s unsaved (w to save)", plural(n, "drop-in")))
	}
	return t
}

func (a *app) render(promptLines []string) []string {
	w, h := a.scr.size()
	keys := keybar(w, mainKeys)
	fixed := 2 + 1 + len(keys)
	if promptLines != nil {
		fixed += len(promptLines) - 1
	}
	listH := max(3, h-fixed)
	lines := []string{a.titleLine()}
	lines = append(lines, dim(fmt.Sprintf(" %-*s %-12s %-9s %-6s %8s  %s", nameW, "UNIT", "ACTIVE", "ENABLED", "CHECK", "RESTARTS", "SINCE")))
	if a.sel < a.off {
		a.off = a.sel
	}
	if a.sel >= a.off+listH {
		a.off = a.sel - listH + 1
	}
	for i := a.off; i < len(a.rows) && i < a.off+listH; i++ {
		l := fit(a.rowLine(a.rows[i]), w)
		if i == a.sel {
			l = inv(padRight(stripSGR(l), w))
		}
		lines = append(lines, l)
	}
	if len(a.rows) == 0 {
		lines = append(lines, dim("   no units match (f: filter, /: search)"))
	}
	for len(lines) < 2+listH {
		lines = append(lines, "")
	}
	if promptLines != nil {
		lines = append(lines, promptLines...)
	} else {
		lines = append(lines, " "+a.status)
	}
	return append(lines, keys...)
}

const nameW = 28

// rowLine formats one list row (not fitted to the screen width).
func (a *app) rowLine(r row) string {
	name := r.u.Name
	if r.child {
		name = "  └ " + name
	}
	restarts := "-"
	if r.u.Kind == "service" {
		restarts = fmt.Sprint(r.u.NRestarts)
	}
	return " " + padRight(fit(name, nameW), nameW) + " " + padRight(activeCell(r.u), 12) + " " +
		padRight(r.u.FileState, 9) + " " + padRight(a.checkCell(r.u), 6) + " " + fmt.Sprintf("%8s", restarts) + "  " + a.sinceCell(r)
}

// stripSGR removes colour codes so the selected row can be shown inverted.
func stripSGR(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && s[j] != 'm' {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// ensureRoot asks for the sudo password when needed; false when there is no root.
func (a *app) ensureRoot() bool {
	if a.h.canRoot() {
		return true
	}
	if a.h.Priv != PrivPassword {
		a.status = red("This needs root, and " + a.h.Login + " has no sudo on this host.")
		return false
	}
	a.scr.suspend()
	fmt.Fprintln(os.Stderr)
	a.h.askPassword("[sudo] password for " + a.h.Login + " on " + a.h.r.Label() + " (empty = cancel): ")
	a.scr.resume()
	if !a.h.canRoot() {
		a.status = yellow("Cancelled: no sudo password.")
		return false
	}
	return true
}

var verbWords = map[string][3]string{ // verb: question, progressive, past
	"start": {"Start", "Starting", "started"}, "stop": {"Stop", "Stopping", "stopped"},
	"restart": {"Restart", "Restarting", "restarted"}, "enable": {"Enable", "Enabling", "enabled"},
	"disable": {"Disable", "Disabling", "disabled"},
}

// doAction asks, runs systemctl <verb> as root, re-reads the unit and shows
// the journal when it ends up failed.
func (a *app) doAction(base func([]string) []string, verb string) {
	u := a.current()
	if u == nil {
		return
	}
	w := verbWords[verb]
	if k := a.ask(base, w[0]+" "+u.Name+"?", "y = yes   any other key = no"); k != 'y' && k != 'Y' {
		a.status = "Cancelled."
		return
	}
	if !a.ensureRoot() {
		return
	}
	a.status = w[1] + " " + u.Name + "…"
	a.scr.draw(base(nil), -1, -1)
	_, err := a.h.action(verb, u.Name)
	names := append([]string{u.Name}, u.Triggers...)
	_ = a.h.refresh(names...)
	a.recheck(u.Name)
	a.rebuild()
	if err != nil {
		a.status = red(verb + " failed: " + err.Error())
	} else {
		a.status = green(u.Name + " " + w[2] + ".")
		if verb == "enable" && a.h.Unit(u.Name).Active != "active" {
			a.status += " Press s to start it now."
		}
	}
	if nu := a.h.Unit(u.Name); nu != nil && nu.Active == "failed" {
		a.showJournal(nu, 10)
	}
}

// recheck re-probes one unit (if it was probed before or details are open).
func (a *app) recheck(name string) {
	u := a.h.Unit(name)
	if u == nil {
		return
	}
	if m, err := a.h.probe([]*Unit{u}); err == nil {
		a.facts[name] = m[name]
	}
}

func (a *app) toggleEnable(base func([]string) []string) {
	u := a.current()
	if u == nil {
		return
	}
	switch u.FileState {
	case "enabled", "enabled-runtime":
		a.doAction(base, "disable")
	case "disabled":
		a.doAction(base, "enable")
	default:
		a.status = u.Name + " is " + u.FileState + "; only enabled/disabled units can be toggled."
	}
}

func (a *app) reload() {
	if a.dirtyCount() > 0 {
		if k := a.ask(a.render, "Reload and discard unsaved drop-in edits?", "y = yes   any other key = no"); k != 'y' && k != 'Y' {
			return
		}
	}
	a.status = "Reloading…"
	a.scr.draw(a.render(nil), -1, -1)
	h, err := loadHost(a.h.r)
	if err != nil {
		a.status = red("reload failed: " + err.Error())
		return
	}
	h.pw, h.journalRoot = a.h.pw, a.h.journalRoot
	a.h = h
	a.edits = map[string]*DropIn{}
	a.probeDefault()
	a.rebuild()
	a.status = "Reloaded " + plural(len(h.Units), "unit") + "."
}

// probeDefault probes the units audit would check.
func (a *app) probeDefault() {
	a.facts = map[string]*UnitFacts{}
	if m, err := a.h.probe(auditUnits(a.h, false)); err == nil {
		a.facts = m
	} else {
		a.status = red("checks failed: " + err.Error())
	}
}

func (a *app) searchPrompt() {
	v, ok := a.prompt(a.render, "Search unit names (empty = clear)", a.search, nil)
	if ok {
		a.search = strings.TrimSpace(v)
		a.rebuild()
		a.selectSearch()
	}
}

// selectSearch moves to the first row whose own name matches the search (a
// timer is listed because its service matches; the service is what was meant).
func (a *app) selectSearch() {
	for i, r := range a.rows {
		if a.search != "" && strings.Contains(r.u.Name, a.search) {
			a.sel = i
			return
		}
	}
}

func (a *app) confirmQuit() bool {
	if a.dirtyCount() == 0 {
		return true
	}
	switch a.ask(a.render, "You have unsaved drop-in edits.", "s = save and quit   d = discard and quit   esc = cancel") {
	case 's', 'S':
		return a.save()
	case 'd', 'D':
		return true
	}
	return false
}

func (a *app) loop() {
	for {
		a.scr.draw(a.render(nil), -1, -1)
		k, ok := a.scr.keys.Read()
		if !ok {
			return
		}
		a.status = ""
		switch {
		case k.Name == "up" || k.Rune == 'k':
			a.sel = max(0, a.sel-1)
		case k.Name == "down" || k.Rune == 'j':
			a.sel = max(0, min(len(a.rows)-1, a.sel+1))
		case k.Name == "pgup":
			a.sel = max(0, a.sel-10)
		case k.Name == "pgdn":
			a.sel = max(0, min(len(a.rows)-1, a.sel+10))
		case k.Name == "home" || k.Rune == 'g':
			a.sel = 0
		case k.Name == "end" || k.Rune == 'G':
			a.sel = max(0, len(a.rows)-1)
		case k.Rune == '/':
			a.searchPrompt()
		case k.Rune == 'f':
			a.filter = (a.filter + 1) % len(filters)
			a.rebuild()
		case k.Rune == 's':
			a.doAction(a.render, "start")
		case k.Rune == 'S':
			a.doAction(a.render, "stop")
		case k.Rune == 'R':
			a.doAction(a.render, "restart")
		case k.Rune == ' ' || k.Rune == 't':
			a.toggleEnable(a.render)
		case k.Rune == 'l':
			if u := a.current(); u != nil {
				a.showJournal(u, 200)
			}
		case k.Name == "enter" || k.Name == "right":
			if a.current() != nil {
				a.details()
			}
		case k.Rune == 'e':
			if u := a.current(); u != nil {
				a.editDropIn(u)
			}
		case k.Rune == 'w':
			a.save()
		case k.Rune == 'c':
			if u := a.current(); u != nil {
				a.showCat(u)
			}
		case k.Rune == 'r':
			a.reload()
		case k.Rune == '?' || k.Rune == 'h':
			a.help()
		case k.Rune == 'q' || k.Name == "ctrl-c" || k.Name == "esc":
			if a.confirmQuit() {
				return
			}
		}
	}
}

func runInteractive(r Runner) error {
	fmt.Fprintf(os.Stderr, "Connecting to %s…\n", r.Label())
	h, err := loadHost(r)
	if err != nil {
		return err
	}
	a := &app{h: h, edits: map[string]*DropIn{}}
	if h.Priv == PrivPassword {
		fmt.Fprintln(os.Stderr, dim("sudo needs a password here; give it now for exact checks, or press enter to skip (svcman asks again when an action needs root)."))
		h.askPassword("[sudo] password for " + h.Login + " on " + r.Label() + ": ")
	}
	fmt.Fprintf(os.Stderr, "Checking %s…\n", plural(len(auditUnits(h, false)), "unit"))
	a.probeDefault()
	a.rows = buildRows(h, 0, "")
	scr, err := openScreen()
	if err != nil {
		return err
	}
	a.scr = scr
	defer scr.close()
	if a.status == "" {
		a.status = "Loaded " + plural(len(h.Units), "unit") + ". Press ? for help."
	}
	a.loop()
	return nil
}
