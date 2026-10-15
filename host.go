// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Priv is how svcman can become root on the host.
type Priv int

const (
	PrivNone     Priv = iota // not root, no sudo
	PrivRoot                 // logged in as root
	PrivNoPass               // passwordless sudo
	PrivPassword             // sudo asks for a password
)

// envC makes systemctl/journalctl output stable and parseable.
const envC = "LC_ALL=C TZ=UTC SYSTEMD_PAGER= SYSTEMD_COLORS=0"

// Host is one target machine and its units.
type Host struct {
	r           Runner
	Login       string
	Home        string
	Priv        Priv
	SysState    string
	Now         time.Time // host clock when loaded
	loadedAt    time.Time // local clock when loaded
	Units       []*Unit
	pw          string
	journalRoot bool
}

const loadScript = `command -v systemctl >/dev/null 2>&1 || { echo @@NOSYSTEMD; exit 0; }
export ` + envC + `
echo @@INFO
date +%s
id -un
echo "$HOME"
if [ "$(id -u)" = 0 ]; then echo root
elif command -v sudo >/dev/null 2>&1 && sudo -n true 2>/dev/null; then echo nopass
elif command -v sudo >/dev/null 2>&1; then echo pass
else echo none; fi
s=$(systemctl is-system-running 2>/dev/null); echo "${s:-unknown}"
echo @@SHOW
{ systemctl list-units --all --type=service,timer --plain --no-legend --no-pager 2>/dev/null
  systemctl list-unit-files --type=service,timer --no-legend --no-pager 2>/dev/null
} | sed 's/^[^a-zA-Z0-9_]* *//' | awk '{print $1}' | grep -E '\.(service|timer)$' | grep -v '@\.' | sort -u |
  tr '\n' '\0' | xargs -0 -r systemctl show --no-pager -p ` + showProps + ` 2>/dev/null
`

// loadHost reads everything svcman shows about the host in one round trip.
func loadHost(r Runner) (*Host, error) {
	out, errOut, err := RunScript(r, "", loadScript)
	if err != nil && !strings.Contains(out, "@@INFO") && !strings.Contains(out, "@@NOSYSTEMD") {
		if m := strings.TrimSpace(errOut); m != "" {
			return nil, fmt.Errorf("%s: %s", r.Label(), m)
		}
		return nil, err
	}
	h, perr := parseLoad(out)
	if perr != nil {
		return nil, fmt.Errorf("%s: %v", r.Label(), perr)
	}
	h.r = r
	return h, nil
}

func parseLoad(out string) (*Host, error) {
	if strings.Contains(out, "@@NOSYSTEMD") {
		return nil, fmt.Errorf("no systemd on this host (svcman needs Linux with systemd 230 or newer)")
	}
	_, rest, ok := strings.Cut(out, "@@INFO\n")
	if !ok {
		return nil, fmt.Errorf("unexpected output from the host")
	}
	info, show, _ := strings.Cut(rest, "@@SHOW\n")
	f := strings.Split(strings.TrimRight(info, "\n"), "\n")
	if len(f) < 5 {
		return nil, fmt.Errorf("unexpected output from the host")
	}
	h := &Host{Login: f[1], Home: f[2], SysState: f[4]}
	if n, err := strconv.ParseInt(f[0], 10, 64); err == nil {
		h.Now = time.Unix(n, 0).UTC()
		h.loadedAt = time.Now()
	}
	h.Priv = map[string]Priv{"root": PrivRoot, "nopass": PrivNoPass, "pass": PrivPassword}[f[3]]
	h.Units = parseShow(show)
	return h, nil
}

// clock is the host's time now: its clock at load plus the time since.
func (h *Host) clock() time.Time {
	if h.loadedAt.IsZero() {
		return h.Now
	}
	return h.Now.Add(time.Since(h.loadedAt))
}

// Unit returns the named unit, or nil.
func (h *Host) Unit(name string) *Unit {
	for _, u := range h.Units {
		if u.Name == name {
			return u
		}
	}
	return nil
}

// refresh re-reads the named units and replaces them in h.Units.
func (h *Host) refresh(names ...string) error {
	var q []string
	for _, n := range names {
		q = append(q, shq(n))
	}
	out, errOut, err := RunScript(h.r, "", envC+" systemctl show --no-pager -p "+showProps+" "+strings.Join(q, " "))
	if err != nil {
		return fmt.Errorf("%v %s", err, strings.TrimSpace(errOut))
	}
	for _, nu := range parseShow(out) {
		replaced := false
		for i, u := range h.Units {
			if u.Name == nu.Name {
				h.Units[i] = nu
				replaced = true
			}
		}
		if !replaced {
			h.Units = append(h.Units, nu)
		}
	}
	return nil
}
