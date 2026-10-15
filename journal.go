// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"strconv"
	"strings"
)

func journalCmd(unit string, n int, follow bool) string {
	c := envC + " journalctl --no-pager -o short-iso -n " + strconv.Itoa(n) + " -u " + shq(unit)
	if follow {
		c += " -f"
	}
	return c
}

// journal returns the unit's last n log lines. When the login user cannot
// see the system journal it retries as root (if that works without asking).
func (h *Host) journal(unit string, n int) ([]string, error) {
	cmd := journalCmd(unit, n, false)
	if !h.journalRoot {
		out, errOut, err := RunScript(h.r, "", cmd)
		limited := strings.Contains(errOut, "not seeing messages") || strings.Contains(errOut, "No journal files") ||
			strings.Contains(strings.ToLower(errOut), "permission")
		if !limited || !h.canRoot() {
			if err != nil && out == "" {
				return nil, err
			}
			return journalLines(out), nil
		}
		h.journalRoot = true
	}
	out, _, err := h.runRoot(cmd)
	if err != nil && out == "" {
		return nil, err
	}
	return journalLines(out), nil
}

func journalLines(out string) []string {
	var ls []string
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if l != "" && l != "-- No entries --" {
			ls = append(ls, l)
		}
	}
	return ls
}

// followCmd is run with Runner.Interactive; it uses an interactive sudo when
// the journal needed root, because the user is at the terminal then.
func (h *Host) followCmd(unit string) string {
	c := journalCmd(unit, 50, true)
	if h.journalRoot && h.Priv != PrivRoot {
		return "sudo sh -c " + shq(c)
	}
	return c
}
