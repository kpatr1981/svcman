// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"fmt"
	"time"
)

// UnitFacts is replaced in Task 7 by the probe's file and user facts.
type UnitFacts struct{}

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
	if u.Kind == "service" && u.FileState == "enabled" && u.Active == "inactive" &&
		len(u.TriggeredBy) == 0 && u.Type != "oneshot" {
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
		if u.Active == "active" && !u.NextElapse.IsZero() && h.Now.Sub(u.NextElapse) > 5*time.Minute {
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

// checkFacts is filled in by Task 7.
func checkFacts(u *Unit, f *UnitFacts) []Issue { return nil }
