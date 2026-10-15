// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"
)

type auditOpts struct{ verbose, fix bool }

// auditUnits picks the units worth checking: enabled, failed, timers and the
// services they trigger, units needing daemon-reload. all = every unit.
func auditUnits(h *Host, all bool) []*Unit {
	var out []*Unit
	for _, u := range h.Units {
		if u.Load == "not-found" && !all {
			continue // only referenced (e.g. by a timer); the referrer reports it
		}
		if all || u.FileState == "enabled" || u.FileState == "enabled-runtime" || u.Active == "failed" ||
			u.Kind == "timer" || len(u.TriggeredBy) > 0 && strings.HasSuffix(u.TriggeredBy[0], ".timer") || u.NeedReload {
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// hostIssues are problems of the host as a whole.
func hostIssues(h *Host) []Issue {
	var is []Issue
	switch h.SysState {
	case "degraded", "maintenance", "stopping":
		is = append(is, Issue{Sev: SevWarn, Text: "system state is " + h.SysState})
	}
	var failed []string
	for _, u := range h.Units {
		if u.Active == "failed" {
			failed = append(failed, u.Name)
		}
		if u.Load != "masked" {
			continue
		}
		for _, v := range h.Units {
			if v.FileState == "enabled" && (containsStr(v.Wants, u.Name) || containsStr(v.Requires, u.Name)) {
				is = append(is, Issue{Sev: SevWarn, Text: u.Name + " is masked but wanted by " + v.Name})
			}
		}
	}
	if len(failed) > 0 {
		is = append(is, Issue{Sev: SevFail, Text: fmt.Sprintf("%s: %s", plural(len(failed), "failed unit"), strings.Join(failed, ", "))})
	}
	return is
}

func containsStr(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func plural(n int, s string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", s)
	}
	return fmt.Sprintf("%d %ss", n, s)
}

// stdinIsTerminal is a variable so tests can simulate a non-interactive run.
var stdinIsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// askPassword asks for the sudo password on the local terminal. An empty
// answer, or no terminal at all, leaves svcman without root.
func (h *Host) askPassword(prompt string) {
	if !stdinIsTerminal() {
		return
	}
	for h.Priv == PrivPassword && h.pw == "" {
		pw, err := readPassword(prompt)
		if err != nil || pw == "" {
			return
		}
		if err := h.setPassword(pw); err != nil {
			fmt.Fprintln(os.Stderr, red(err.Error()))
		}
	}
}

func runAudit(r Runner, o auditOpts) (int, error) {
	fmt.Fprintf(os.Stderr, "Connecting to %s…\n", r.Label())
	h, err := loadHost(r)
	if err != nil {
		return 2, err
	}
	switch h.Priv {
	case PrivPassword:
		h.askPassword("[sudo] password for " + h.Login + " on " + r.Label() + " (empty = continue without root): ")
	case PrivNone:
		fmt.Fprintln(os.Stderr, yellow("warning: not root and no sudo; file checks are judged from permission bits."))
	}
	if !h.canRoot() && h.Priv != PrivNone {
		fmt.Fprintln(os.Stderr, yellow("warning: continuing without root; file checks are judged from permission bits."))
	}
	units := auditUnits(h, o.verbose)
	fmt.Fprintf(os.Stderr, "Checking %s…\n", plural(len(units), "unit"))
	facts, err := h.probe(units)
	if err != nil {
		return 2, err
	}

	fmt.Printf("%s %s  %s\n", inv(bold(" svcman audit ")), bold(h.Login+"@"+hostOf(r.Label())),
		dim(fmt.Sprintf("systemd: %s · %d units, %d checked", h.SysState, len(h.Units), len(units))))
	var fixes []Fix
	printIssue := func(indent string, i Issue) {
		fmt.Printf("%s%s %s %s\n", indent, sevMark(i.Sev), sevLabel(i.Sev), i.Text)
		for _, f := range i.Fixes {
			fixes = append(fixes, f)
			fmt.Printf("%s       %s %s  %s\n", indent, cyan(fmt.Sprintf("[%d]", len(fixes))), f.Cmd, dim(f.Label))
		}
	}
	for _, i := range hostIssues(h) {
		printIssue("", i)
	}
	failing := failingCount(h, facts)
	for _, u := range units {
		is := Check(h, u, facts[u.Name])
		w := worst(is)
		if w == SevOK && !o.verbose {
			continue
		}
		fmt.Printf("\n%s  %s\n", bold(u.Name), dim("("+u.Active+", "+u.FileState+")"))
		if len(is) == 0 {
			fmt.Println("  " + sevMark(SevOK) + " " + sevLabel(SevOK))
		}
		for _, i := range is {
			printIssue("  ", i)
		}
		if u.Active == "failed" {
			if ls, err := h.journal(u.Name, 5); err == nil && len(ls) > 0 {
				fmt.Println(dim("    last log lines:"))
				for _, l := range ls {
					fmt.Println(dim("      " + l))
				}
			}
		}
	}
	fmt.Println()
	if failing == 0 {
		fmt.Println(green("No failing units."))
	} else {
		fmt.Println(red(plural(failing, "failing unit") + "."))
	}
	if o.fix && len(fixes) > 0 {
		applyFixes(h, fixes)
	}
	if failing > 0 {
		return 1, nil
	}
	return 0, nil
}

// failingCount is the verdict: failing units among those a default audit
// checks, so -v (which lists every unit) does not change the exit status.
func failingCount(h *Host, facts map[string]*UnitFacts) int {
	n := 0
	for _, u := range auditUnits(h, false) {
		if worst(Check(h, u, facts[u.Name])) == SevFail {
			n++
		}
	}
	return n
}

func applyFixes(h *Host, fixes []Fix) {
	for n, f := range fixes {
		a, err := readLine(fmt.Sprintf("\nApply [%d] %s ? [y/N/q] ", n+1, f.Cmd))
		if err != nil || strings.HasPrefix(strings.ToLower(a), "q") {
			return
		}
		if !strings.HasPrefix(strings.ToLower(a), "y") {
			continue
		}
		out, errOut, err := h.runRoot(f.Cmd + " 2>&1")
		switch {
		case err == errNeedPassword || err == errNoRoot:
			fmt.Println(red("  not applied: " + err.Error()))
		case err != nil:
			fmt.Println(red("  failed: " + oneLine(out+" "+errOut)))
		default:
			fmt.Println(green("  done"))
		}
	}
}
func hostOf(label string) string {
	if i := strings.LastIndex(label, "@"); i >= 0 {
		return label[i+1:]
	}
	return label
}
