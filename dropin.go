// svcman — Author: Konstantinos Patronas <kpatronas@gmail.com>

package main

import (
	"errors"
	"fmt"
	"strings"
)

const unitDir = "/etc/systemd/system"

const dropInHeader = "# Managed by svcman. Delete this file to undo.\n"

// Entry is one Key=value line of a drop-in.
type Entry struct{ Section, Key, Value string }

// DropIn is svcman's own drop-in for one unit.
type DropIn struct {
	Unit    string
	Path    string
	Orig    string // content as loaded ("" when there was no file)
	Exists  bool
	Entries []Entry
	base    string // Render() of the loaded entries
}

func dropInPath(unit string) string { return unitDir + "/" + unit + ".d/svcman.conf" }

// isResetKey: settings that add to the vendor value unless reset first.
func isResetKey(k string) bool {
	switch k {
	case "ExecStartPre", "ExecStart", "ExecStartPost", "ExecReload", "ExecStop", "ExecStopPost":
		return true
	}
	return false
}

func parseDropIn(content string) []Entry {
	var es []Entry
	sec := ""
	for _, l := range strings.Split(content, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, ";"):
		case strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]"):
			sec = l[1 : len(l)-1]
		default:
			k, v, ok := strings.Cut(l, "=")
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			if !ok || (v == "" && isResetKey(k)) {
				continue // svcman writes the reset lines itself
			}
			es = append(es, Entry{sec, k, v})
		}
	}
	return es
}

func newDropIn(unit, content string, exists bool) *DropIn {
	d := &DropIn{Unit: unit, Path: dropInPath(unit), Orig: content, Exists: exists, Entries: parseDropIn(content)}
	d.base = d.Render()
	return d
}

// Render writes the entries grouped by section in first-seen order. The first
// value of an Exec* key in a section is preceded by an empty reset line.
func (d *DropIn) Render() string {
	if len(d.Entries) == 0 {
		return ""
	}
	var sections []string
	by := map[string][]Entry{}
	for _, e := range d.Entries {
		if _, ok := by[e.Section]; !ok {
			sections = append(sections, e.Section)
		}
		by[e.Section] = append(by[e.Section], e)
	}
	var b strings.Builder
	b.WriteString(dropInHeader)
	for i, s := range sections {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("[" + s + "]\n")
		reset := map[string]bool{}
		for _, e := range by[s] {
			if isResetKey(e.Key) && !reset[e.Key] {
				b.WriteString(e.Key + "=\n")
				reset[e.Key] = true
			}
			b.WriteString(e.Key + "=" + e.Value + "\n")
		}
	}
	return b.String()
}

func (d *DropIn) Dirty() bool { return d.Render() != d.base }

// knownKey: unit-file keys whose systemctl show property has another name
// (RestartSec→RestartUSec, EnvironmentFile→EnvironmentFiles) count as known.
func knownKey(props map[string]bool, key string) bool {
	if props[key] || props[key+"s"] {
		return true
	}
	if strings.HasSuffix(key, "Sec") && props[strings.TrimSuffix(key, "Sec")+"USec"] {
		return true
	}
	return false
}

func defaultSection(u *Unit) string {
	if u.Kind == "timer" {
		return "Timer"
	}
	return "Service"
}

// loadDropIn reads svcman.conf for unit (world-readable, no root needed).
func loadDropIn(h *Host, unit string) (*DropIn, error) {
	p := shq(dropInPath(unit))
	out, _, err := RunScript(h.r, "", "if [ -e "+p+" ]; then echo @@YES; cat "+p+"; else echo @@NO; fi")
	if err != nil {
		return nil, err
	}
	if rest, ok := strings.CutPrefix(out, "@@YES\n"); ok {
		return newDropIn(unit, rest, true), nil
	}
	return newDropIn(unit, "", false), nil
}

var errChanged = errors.New("changed on the host since it was loaded; reload (r) and edit again")

// saveScript guards against concurrent changes, backs up, writes atomically,
// reloads systemd, and rolls back if the reload fails. dir is unitDir except
// in tests. Output: "CHANGED", "BACKUP <path>", "BACKUPFAIL", "WRITEFAIL",
// "RELOADFAIL" or "OK".
func saveScript(dir string, d *DropIn, content, backupDir, owner, stamp string) string {
	ddir := dir + "/" + d.Unit + ".d"
	es, eb := heredoc(d.Orig)
	ns, nb := heredoc(content)
	existed := "0"
	if d.Exists {
		existed = "1"
	}
	return `f=` + shq(ddir+"/svcman.conf") + `; d=` + shq(ddir) + `; bk=` + shq(backupDir) + `; existed=` + existed + `
exp=$(mktemp) new=$(mktemp) cur=$(mktemp)
trap 'rm -f "$exp" "$new" "$cur"' EXIT
cat > "$exp" ` + es + "\n" + eb + `cat > "$new" ` + ns + "\n" + nb + `if [ -e "$f" ]; then
  [ "$existed" = 1 ] || { echo CHANGED; exit 3; }
  { cat "$f"; [ -n "$(tail -c1 "$f")" ] && echo; } > "$cur"
  cmp -s "$cur" "$exp" || { echo CHANGED; exit 3; }
elif [ "$existed" = 1 ]; then echo CHANGED; exit 3; fi
saved=
if [ -e "$f" ]; then
  saved="$bk/` + d.Unit + `-` + stamp + `.conf"
  mkdir -p "$bk" && cp -p "$f" "$saved" || { echo BACKUPFAIL; exit 4; }
  chown -R ` + shq(owner) + ` "$bk" 2>/dev/null || true
  echo "BACKUP $saved"
fi
if [ -s "$new" ]; then
  mkdir -p "$d" && cp "$new" "$d/.svcman.conf.tmp" && chmod 644 "$d/.svcman.conf.tmp" || { echo WRITEFAIL; exit 4; }
  chown 0:0 "$d/.svcman.conf.tmp" 2>/dev/null || true
  mv "$d/.svcman.conf.tmp" "$f" || { echo WRITEFAIL; exit 4; }
else
  rm -f "$f"; rmdir "$d" 2>/dev/null || true
fi
if ! systemctl daemon-reload >/dev/null 2>&1; then
  if [ -n "$saved" ]; then cp -p "$saved" "$f"; else rm -f "$f"; rmdir "$d" 2>/dev/null || true; fi
  systemctl daemon-reload >/dev/null 2>&1
  echo RELOADFAIL; exit 5
fi
echo OK
`
}

// saveDropIn writes d on the host as root; returns the backup path ("" when
// there was no previous file).
func saveDropIn(h *Host, d *DropIn, stamp string) (string, error) {
	out, errOut, err := h.runRoot(saveScript(unitDir, d, d.Render(), h.Home+"/.svcman_backups", h.Login, stamp))
	backup := ""
	for _, l := range strings.Split(out, "\n") {
		if p, ok := strings.CutPrefix(l, "BACKUP "); ok {
			backup = p
		}
	}
	switch {
	case strings.Contains(out, "CHANGED"):
		return "", errChanged
	case strings.Contains(out, "RELOADFAIL"):
		return backup, errors.New("daemon-reload failed; the previous drop-in was restored")
	case strings.Contains(out, "BACKUPFAIL"):
		return "", errors.New("could not back up " + d.Path + "; nothing written")
	case strings.Contains(out, "WRITEFAIL"):
		return backup, errors.New("could not write " + d.Path)
	case strings.Contains(out, "OK"):
		return backup, nil
	}
	if err == nil {
		err = errors.New("unexpected output")
	}
	return backup, fmt.Errorf("%v %s", err, oneLine(errOut))
}
