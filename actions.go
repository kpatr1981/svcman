// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"errors"
	"strings"
)

// action runs "systemctl <verb> <units>" as root. On failure the error text
// is systemctl's own message on one line.
func (h *Host) action(verb string, units ...string) (string, error) {
	cmd := envC + " systemctl " + verb
	for _, u := range units {
		cmd += " " + shq(u)
	}
	out, errOut, err := h.runRoot(cmd + " 2>&1")
	if err != nil {
		if errors.Is(err, errNeedPassword) || errors.Is(err, errNoRoot) {
			return "", err
		}
		if m := oneLine(out + " " + errOut); m != "" {
			return out, errors.New(m)
		}
		return out, err
	}
	return out, nil
}

// oneLine turns command output into a one-line message, without the noise
// login shells may print on every command.
func oneLine(s string) string { return strings.Join(strings.Fields(cleanStderr(s)), " ") }

// cleanStderr drops lines that come from the host's login environment rather
// than from the command (locale warnings from bash/sh on misconfigured hosts).
func cleanStderr(s string) string {
	var keep []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, "setlocale:") || strings.Contains(l, "cannot change locale") {
			continue
		}
		keep = append(keep, l)
	}
	return strings.Join(keep, "\n")
}
