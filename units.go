// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// showProps are the properties svcman reads with systemctl show.
const showProps = "Id,Description,Type,LoadState,ActiveState,SubState,Result,UnitFileState," +
	"FragmentPath,DropInPaths,ExecStartPre,ExecStart,ExecStartPost,ExecReload,ExecStop,ExecStopPost," +
	"User,Group,DynamicUser,WorkingDirectory,EnvironmentFiles,NRestarts,Restart," +
	"Triggers,TriggeredBy,Wants,Requires,StateChangeTimestamp,NextElapseUSecRealtime,LastTriggerUSec,NeedDaemonReload"

var execKeys = []string{"ExecStartPre", "ExecStart", "ExecStartPost", "ExecReload", "ExecStop", "ExecStopPost"}

// ExecCmd is one command of an Exec*= setting.
type ExecCmd struct {
	Key          string // ExecStart, ExecStartPre, ...
	Path         string // the executable systemd runs
	Argv         string // the command line as systemd shows it
	IgnoreErrors bool   // "-" prefix
}

// EnvFile is one EnvironmentFile= entry.
type EnvFile struct {
	Path     string
	Optional bool // "-" prefix
}

// Unit is a service or timer as systemctl show describes it.
type Unit struct {
	Name, Kind, Description, Type        string
	Load, Active, Sub, Result, FileState string
	Fragment                             string
	DropIns                              []string
	Exec                                 []ExecCmd
	User, Group                          string
	DynamicUser                          bool
	WorkDir                              string
	EnvFiles                             []EnvFile
	NRestarts                            int
	Restart                              string
	Triggers, TriggeredBy                []string
	Wants, Requires                      []string
	StateChange, NextElapse, LastTrigger time.Time
	NeedReload                           bool
}

// parseShow splits systemctl show output (blank-line separated key=value
// blocks) into units. Blocks without Id= are dropped, and so are repeats of
// an Id (an alias such as sshd.service shows its unit again).
func parseShow(out string) []*Unit {
	var units []*Unit
	var cur map[string][]string
	seen := map[string]bool{}
	flush := func() {
		if id := cur["Id"]; len(id) > 0 && !seen[id[0]] {
			seen[id[0]] = true
			units = append(units, unitFromProps(cur))
		}
		cur = nil
	}
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimRight(l, "\r")
		if l == "" {
			flush()
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		if cur == nil {
			cur = map[string][]string{}
		}
		cur[k] = append(cur[k], v)
	}
	flush()
	return units
}

func unitFromProps(p map[string][]string) *Unit {
	get := func(k string) string {
		if v := p[k]; len(v) > 0 {
			return v[len(v)-1]
		}
		return ""
	}
	u := &Unit{
		Name: get("Id"), Description: get("Description"), Type: get("Type"),
		Load: get("LoadState"), Active: get("ActiveState"), Sub: get("SubState"),
		Result: get("Result"), FileState: get("UnitFileState"), Fragment: get("FragmentPath"),
		DropIns: strings.Fields(get("DropInPaths")),
		User:    get("User"), Group: get("Group"), DynamicUser: get("DynamicUser") == "yes",
		WorkDir: get("WorkingDirectory"), Restart: get("Restart"),
		Triggers: strings.Fields(get("Triggers")), TriggeredBy: strings.Fields(get("TriggeredBy")),
		Wants: strings.Fields(get("Wants")), Requires: strings.Fields(get("Requires")),
		StateChange: parseTime(get("StateChangeTimestamp")),
		NextElapse:  parseTime(get("NextElapseUSecRealtime")),
		LastTrigger: parseTime(get("LastTriggerUSec")),
		NeedReload:  get("NeedDaemonReload") == "yes",
	}
	u.Kind = strings.TrimPrefix(path.Ext(u.Name), ".")
	u.NRestarts, _ = strconv.Atoi(get("NRestarts"))
	for _, k := range execKeys {
		for _, v := range p[k] {
			u.Exec = append(u.Exec, parseExec(k, v)...)
		}
	}
	for _, v := range p["EnvironmentFiles"] {
		f, rest, _ := strings.Cut(v, " ")
		if f != "" {
			u.EnvFiles = append(u.EnvFiles, EnvFile{Path: f, Optional: strings.Contains(rest, "ignore_errors=yes")})
		}
	}
	return u
}

// argv[]= runs up to " ; ignore_errors=", so arguments containing " ; " survive.
var execRe = regexp.MustCompile(`path=(\S+) ; argv\[\]=(.*?) ; ignore_errors=(yes|no)`)

// parseExec reads one Exec*= value, which may hold several { ... } commands.
func parseExec(key, v string) []ExecCmd {
	var out []ExecCmd
	for _, m := range execRe.FindAllStringSubmatch(v, -1) {
		out = append(out, ExecCmd{Key: key, Path: m[1], Argv: m[2], IgnoreErrors: m[3] == "yes"})
	}
	return out
}

// parseTime reads a systemd timestamp printed with TZ=UTC LC_ALL=C.
func parseTime(s string) time.Time {
	t, err := time.Parse("Mon 2006-01-02 15:04:05 MST", s)
	if err != nil {
		return time.Time{}
	}
	return t
}
