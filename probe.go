// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"fmt"
	"strconv"
	"strings"
)

type probeReq struct {
	Unit, Role string
	Kind       byte
	Path, User string
	Optional   bool
}

// Fact is what the probe found about one probeReq.
type Fact struct {
	Req          probeReq
	Exists       bool
	State        string
	Type         string
	Mode         int
	Owner, Group string
	Access       string
	AccessR      string
	Interp       string
	InterpOK     bool
	EnvCmd       string
	EnvCmdOK     bool
	CRLF, NoExec bool
}

// UnitFacts is everything the probe found for one unit.
type UnitFacts struct {
	Facts  []Fact
	Verify []string
}

// requests lists what to check for u, as the user the unit runs as.
func requests(u *Unit) []probeReq {
	user := u.User
	if user == "" || u.DynamicUser {
		user = "root"
	}
	var rs []probeReq
	add := func(kind byte, role, p, usr string, opt bool) {
		rs = append(rs, probeReq{Unit: u.Name, Role: role, Kind: kind, Path: p, User: usr, Optional: opt})
	}
	for _, e := range u.Exec {
		add('x', e.Key, e.Path, user, e.IgnoreErrors)
	}
	if wd := u.WorkDir; wd != "" && wd != "~" {
		opt := strings.HasPrefix(wd, "-")
		wd = strings.TrimLeft(wd, "-!")
		if strings.HasPrefix(wd, "/") {
			add('d', "WorkingDirectory", wd, user, opt)
		}
	}
	for _, f := range u.EnvFiles {
		add('r', "EnvironmentFile", f.Path, user, f.Optional)
	}
	if u.Fragment != "" {
		add('f', "unit file", u.Fragment, "root", false)
	}
	for _, d := range u.DropIns {
		add('f', "drop-in", d, "root", false)
	}
	if u.User != "" && !u.DynamicUser {
		add('u', "User", u.User, "", false)
	}
	if u.Group != "" && !u.DynamicUser {
		add('g', "Group", u.Group, "", false)
	}
	if strings.HasPrefix(u.Fragment, "/etc/systemd/system/") {
		add('s', "vendor", u.Name, "root", false)
	}
	if u.Fragment != "" {
		add('v', "verify", u.Fragment, "root", false)
	}
	return rs
}

const probeFuncs = `export LC_ALL=C
ME=$(id -u); MEN=$(id -un)
st() { stat -L -c '%a %U %G' "$1" 2>/dev/null || stat -L -f '%Lp %Su %Sg' "$1" 2>/dev/null; }
as() { # user test-op path
  if [ "$1" = "$MEN" ]; then test $2 "$3" && echo ok || echo denied; return; fi
  if [ "$ME" = 0 ]; then
    if command -v runuser >/dev/null 2>&1; then runuser -u "$1" -- test $2 "$3" 2>/dev/null && echo ok || echo denied
    else su -s /bin/sh "$1" -c 'test "$0" "$1"' "$2" "$3" 2>/dev/null && echo ok || echo denied; fi
    return
  fi
  test $2 "$3" && echo approx-ok || echo approx-denied
}
CR=$(printf '\r')
while IFS='	' read -r idx kind path user; do
  case $kind in
  u) id -u "$path" >/dev/null 2>&1 && e=1 || e=0; printf 'F\t%s\texists=%s\n' "$idx" "$e"; continue ;;
  g) { getent group "$path" || grep -q "^$path:" /etc/group; } >/dev/null 2>&1 && e=1 || e=0; printf 'F\t%s\texists=%s\n' "$idx" "$e"; continue ;;
  s) e=0; for v in /usr/lib/systemd/system /lib/systemd/system; do [ -e "$v/$path" ] && [ ! "$v/$path" -ef "/etc/systemd/system/$path" ] && e=1; done
     printf 'F\t%s\texists=%s\n' "$idx" "$e"; continue ;;
  v) command -v systemd-analyze >/dev/null 2>&1 || continue
     systemd-analyze verify "$path" 2>&1 | grep -v '^$' | head -5 | while IFS= read -r l; do printf 'V\t%s\t%s\n' "$idx" "$l"; done
     continue ;;
  esac
  if [ "$kind" = x ]; then # bare command: systemd searches its own path
    case $path in /*) ;; *) r=$( (PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; command -v "$path") 2>/dev/null)
      case $r in /*) path=$r ;; esac ;; esac
  fi
  if [ ! -e "$path" ]; then
    if [ -L "$path" ]; then s=broken-link; else s=missing; fi
    printf 'F\t%s\texists=0 state=%s\n' "$idx" "$s"; continue
  fi
  set -- $(st "$path"); mode=$1 own=$2 grp=$3
  if [ -d "$path" ]; then t=dir; elif [ -f "$path" ]; then t=file; else t=other; fi
  x=
  case $kind in x|d) x="access=$(as "$user" -x "$path")" ;; r) x="access=$(as "$user" -r "$path")" ;; esac
  if [ "$kind" = x ] && [ "$t" = file ]; then
    l1=$(head -n1 "$path" 2>/dev/null | head -c 200)
    case $l1 in '#!'*)
      i=$(printf '%s' "$l1" | sed 's/^#! *//; s/[ '"$CR"'].*//')
      if [ -x "$i" ]; then x="$x interp=$i interp_ok=1"; else x="$x interp=$i interp_ok=0"; fi
      if [ "$i" = /usr/bin/env ]; then
        a=$(printf '%s' "$l1" | tr -d "$CR" | awk '{print $2}')
        if (PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin; command -v "$a") >/dev/null 2>&1; then o=1; else o=0; fi
        x="$x envcmd=$a envcmd_ok=$o"
      fi
      head -c 4096 "$path" 2>/dev/null | grep -q "$CR" && x="$x crlf=1"
      x="$x access_r=$(as "$user" -r "$path")" ;;
    esac
    if command -v findmnt >/dev/null 2>&1 && findmnt -n -o OPTIONS -T "$path" 2>/dev/null | tr , '\n' | grep -qx noexec; then x="$x noexec=1"; fi
  fi
  printf 'F\t%s\texists=1 type=%s mode=%s owner=%s group=%s %s\n' "$idx" "$t" "$mode" "$own" "$grp" "$x"
done `

// probeScript returns a sh script that checks every request; one output line
// per request ("F\t<idx>\tkey=value ...") plus "V\t<idx>\t<line>" for verify.
func probeScript(reqs []probeReq) string {
	var data strings.Builder
	for i, r := range reqs {
		fmt.Fprintf(&data, "%d\t%c\t%s\t%s\n", i, r.Kind, r.Path, r.User)
	}
	start, body := heredoc(data.String())
	return probeFuncs + start + "\n" + body
}

func parseProbe(out string, reqs []probeReq) map[string]*UnitFacts {
	m := map[string]*UnitFacts{}
	get := func(i int) *UnitFacts {
		u := reqs[i].Unit
		if m[u] == nil {
			m[u] = &UnitFacts{}
		}
		return m[u]
	}
	for _, l := range strings.Split(out, "\n") {
		parts := strings.SplitN(l, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		i, err := strconv.Atoi(parts[1])
		if err != nil || i < 0 || i >= len(reqs) {
			continue
		}
		switch parts[0] {
		case "V":
			uf := get(i)
			uf.Verify = append(uf.Verify, parts[2])
		case "F":
			f := Fact{Req: reqs[i]}
			for _, kv := range strings.Fields(parts[2]) {
				k, v, _ := strings.Cut(kv, "=")
				switch k {
				case "exists":
					f.Exists = v == "1"
				case "state":
					f.State = v
				case "type":
					f.Type = v
				case "mode":
					n, _ := strconv.ParseInt(v, 8, 32)
					f.Mode = int(n)
				case "owner":
					f.Owner = v
				case "group":
					f.Group = v
				case "access":
					f.Access = v
				case "access_r":
					f.AccessR = v
				case "interp":
					f.Interp = v
				case "interp_ok":
					f.InterpOK = v == "1"
				case "envcmd":
					f.EnvCmd = v
				case "envcmd_ok":
					f.EnvCmdOK = v == "1"
				case "crlf":
					f.CRLF = v == "1"
				case "noexec":
					f.NoExec = v == "1"
				}
			}
			uf := get(i)
			uf.Facts = append(uf.Facts, f)
		}
	}
	return m
}

// probe checks the files and users of units, as root when that works
// without asking (exact per-user tests), otherwise as the login user
// (access results are then marked approx-).
func (h *Host) probe(units []*Unit) (map[string]*UnitFacts, error) {
	var reqs []probeReq
	for _, u := range units {
		reqs = append(reqs, requests(u)...)
	}
	if len(reqs) == 0 {
		return map[string]*UnitFacts{}, nil
	}
	script := probeScript(reqs)
	var out string
	var err error
	if h.canRoot() {
		out, _, err = h.runRoot(script)
	} else {
		out, _, err = RunScript(h.r, "", script)
	}
	if err != nil && out == "" {
		return nil, err
	}
	m := parseProbe(out, reqs)
	for _, u := range units { // a unit with no findings still counts as probed
		if m[u.Name] == nil {
			m[u.Name] = &UnitFacts{}
		}
	}
	return m, nil
}
