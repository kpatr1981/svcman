// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var keyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)

// parseEntryInput reads "Key=value" or "Section/Key=value".
func parseEntryInput(s, defSection string) (Entry, string) {
	k, v, ok := strings.Cut(strings.TrimSpace(s), "=")
	k, v = strings.TrimSpace(k), strings.TrimSpace(v)
	if !ok || k == "" {
		return Entry{}, "write Key=value (or Section/Key=value)"
	}
	sec := defSection
	if a, b, ok := strings.Cut(k, "/"); ok {
		sec, k = strings.TrimSpace(a), strings.TrimSpace(b)
	}
	if !keyRe.MatchString(k) || !keyRe.MatchString(sec) {
		return Entry{}, "a key is letters and digits, e.g. Restart=always"
	}
	return Entry{sec, k, v}, ""
}

// diffDropIn shows old and new content of one drop-in.
func diffDropIn(d *DropIn) []string {
	out := []string{bold(d.Path)}
	newC := d.Render()
	oldL := strings.Split(strings.TrimRight(d.Orig, "\n"), "\n")
	newL := strings.Split(strings.TrimRight(newC, "\n"), "\n")
	inOld := map[string]bool{}
	for _, l := range oldL {
		inOld[l] = true
	}
	inNew := map[string]bool{}
	for _, l := range newL {
		inNew[l] = true
	}
	for _, l := range oldL {
		if l != "" && !inNew[l] {
			out = append(out, red(" - "+l))
		}
	}
	for _, l := range newL {
		if l != "" && !inOld[l] {
			out = append(out, green(" + "+l))
		}
	}
	if newC == "" {
		out = append(out, yellow(" (file will be deleted)"))
	}
	return append(out, "")
}

// propNames lists every property systemd knows for u (to warn about typos).
func (a *app) propNames(u *Unit) map[string]bool {
	out, _, _ := RunScript(a.h.r, "", envC+" systemctl show --all --no-pager "+shq(u.Name)+" | cut -d= -f1")
	m := map[string]bool{}
	for _, l := range strings.Fields(out) {
		m[l] = true
	}
	return m
}

var editorKeys = [][2]string{{"a", "add"}, {"enter", "change value"}, {"D", "delete"}, {"w", "save"}, {"esc/q", "back"}}

func (a *app) editDropIn(u *Unit) {
	d := a.edits[u.Name]
	if d == nil {
		var err error
		if d, err = loadDropIn(a.h, u.Name); err != nil {
			a.status = red("cannot read drop-in: " + err.Error())
			return
		}
		a.edits[u.Name] = d
	}
	props := a.propNames(u)
	sel := 0
	for {
		w, h := a.scr.size()
		lines := []string{inv(bold(" Drop-in ")) + " " + bold(u.Name) + "  " + dim(d.Path), ""}
		if len(d.Entries) == 0 {
			lines = append(lines, dim("   (empty — press a to add a setting, e.g. Restart=always)"))
		}
		for i, e := range d.Entries {
			l := fmt.Sprintf("   [%s] %s=%s", e.Section, e.Key, e.Value)
			if !knownKey(props, e.Key) && len(props) > 0 {
				l += "  " + yellow("(unknown to systemd?)")
			}
			if isResetKey(e.Key) {
				l += "  " + dim("(replaces the unit's "+e.Key+")")
			}
			if i == sel {
				l = inv(padRight(stripSGR(fit(l, w)), w))
			}
			lines = append(lines, fit(l, w))
		}
		var others []string
		for _, p := range u.DropIns {
			if p != d.Path {
				others = append(others, p)
			}
		}
		if len(others) > 0 {
			lines = append(lines, "", dim("   other drop-ins (read-only, see c): "+strings.Join(others, " ")))
		}
		bar := keybar(w, editorKeys)
		for len(lines) < h-len(bar)-1 {
			lines = append(lines, "")
		}
		base := func(p []string) []string {
			out := append([]string{}, lines[:min(len(lines), h-len(bar)-len(p))]...)
			out = append(out, p...)
			return append(out, bar...)
		}
		a.scr.draw(base([]string{" " + a.status}), -1, -1)
		k, ok := a.scr.keys.Read()
		if !ok {
			return
		}
		a.status = ""
		validate := func(s string) string {
			_, msg := parseEntryInput(s, defaultSection(u))
			return msg
		}
		switch {
		case k.Name == "up" || k.Rune == 'k':
			sel = max(0, sel-1)
		case k.Name == "down" || k.Rune == 'j':
			sel = max(0, min(len(d.Entries)-1, sel+1))
		case k.Rune == 'a':
			if v, ok := a.prompt(base, "Add setting: Key=value or Section/Key=value  ["+defaultSection(u)+"]", "", validate); ok {
				e, _ := parseEntryInput(v, defaultSection(u))
				d.Entries = append(d.Entries, e)
				sel = len(d.Entries) - 1
				if !knownKey(props, e.Key) && len(props) > 0 {
					a.status = yellow(e.Key + " is not a property systemd reports for this unit; check the spelling.")
				}
			}
		case (k.Name == "enter" || k.Rune == 'c') && sel < len(d.Entries):
			e := d.Entries[sel]
			if v, ok := a.prompt(base, "Change setting", e.Section+"/"+e.Key+"="+e.Value, validate); ok {
				d.Entries[sel], _ = parseEntryInput(v, defaultSection(u))
			}
		case k.Rune == 'D' && sel < len(d.Entries):
			d.Entries = append(d.Entries[:sel], d.Entries[sel+1:]...)
			sel = max(0, min(sel, len(d.Entries)-1))
		case k.Rune == 'w':
			a.save()
		case k.Rune == 'q' || k.Name == "esc" || k.Name == "ctrl-c" || k.Name == "left":
			if d.Dirty() {
				a.status = yellow("Drop-in for " + u.Name + " edited; press w to save.")
			}
			return
		}
	}
}

// save writes every edited drop-in after one confirmation. Returns true when
// nothing is left unsaved.
func (a *app) save() bool {
	var dirty []*DropIn
	var diff []string
	for _, d := range a.edits {
		if d.Dirty() {
			dirty = append(dirty, d)
			diff = append(diff, diffDropIn(d)...)
		}
	}
	if len(dirty) == 0 {
		a.status = "No changes."
		return true
	}
	confirmed := false
	a.pager("Pending drop-in changes", diff, [][2]string{{"y", "write and daemon-reload"}}, false, func(k Key) bool {
		if k.Rune == 'y' || k.Rune == 'Y' {
			confirmed = true
			return true
		}
		return false
	})
	if !confirmed {
		a.status = "Not saved."
		return false
	}
	if !a.ensureRoot() {
		return false
	}
	stamp := time.Now().Format("20060102-150405")
	var msgs []string
	var saved []string
	ok := true
	for _, d := range dirty {
		bk, err := saveDropIn(a.h, d, stamp)
		if err != nil {
			ok = false
			msgs = append(msgs, red(d.Unit+": "+err.Error()))
			continue
		}
		m := green(d.Unit + ": saved")
		if bk != "" {
			m += dim("  backup: " + bk)
		}
		msgs = append(msgs, m)
		saved = append(saved, d.Unit)
		d.markSaved() // an editor still open on d keeps working against the saved state
	}
	if len(saved) > 0 {
		_ = a.h.refresh(saved...)
		for _, n := range saved {
			a.recheck(n)
		}
		a.rebuild()
		var running []string
		for _, n := range saved {
			if u := a.h.Unit(n); u != nil && u.Active == "active" && u.Kind == "service" {
				running = append(running, n)
			}
		}
		a.status = strings.Join(msgs, "  ")
		if len(running) > 0 {
			if k := a.ask(a.render, "Restart "+strings.Join(running, ", ")+" so the change takes effect?", "y = restart   any other key = later"); k == 'y' || k == 'Y' {
				if _, err := a.h.action("restart", running...); err != nil {
					msgs = append(msgs, red("restart failed: "+err.Error()))
				} else {
					msgs = append(msgs, green("restarted"))
				}
				_ = a.h.refresh(running...)
				a.rebuild()
			}
		}
	}
	a.status = strings.Join(msgs, "  ")
	return ok
}
