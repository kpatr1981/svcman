// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"errors"
	"fmt"
	"strings"
)

var (
	errNeedPassword = errors.New("needs the sudo password")
	errNoRoot       = errors.New("needs root, and sudo is not available")
)

// canRoot reports whether runRoot works without asking for anything.
func (h *Host) canRoot() bool {
	return h.Priv == PrivRoot || h.Priv == PrivNoPass || (h.Priv == PrivPassword && h.pw != "")
}

// runRoot runs a sh script as root: directly, with sudo -n, or with the sudo
// password typed earlier on stdin. sudo never prompts on the remote side.
// The script always travels on stdin (sudo -S reads only the password line),
// because a single argument is limited to 128 KiB on Linux.
func (h *Host) runRoot(script string) (string, string, error) {
	switch {
	case h.Priv == PrivRoot:
		return RunScript(h.r, "", script)
	case h.Priv == PrivNoPass:
		return RunScript(h.r, "sudo -n", script)
	case h.Priv == PrivPassword && h.pw != "":
		return h.r.Exec("sudo -k -S -p '' /bin/sh -s", h.pw+"\n"+script)
	case h.Priv == PrivPassword:
		return "", "", errNeedPassword
	}
	return "", "", errNoRoot
}

// setPassword checks pw with sudo and keeps it for later root commands.
func (h *Host) setPassword(pw string) error {
	out, _, _ := h.r.Exec("sudo -k -S -p '' sh -c 'echo SUDO_OK'", pw+"\n")
	if !strings.Contains(out, "SUDO_OK") {
		return fmt.Errorf("sudo authentication failed on %s", h.r.Label())
	}
	h.pw = pw
	return nil
}
