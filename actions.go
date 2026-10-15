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

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
