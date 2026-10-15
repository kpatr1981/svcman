# svcman — design

Date: 2026-10-15
Status: approved in conversation, awaiting written-spec review

## Purpose

Manage and audit systemd services and timers on remote hosts over SSH, in the
same fashion as `cronman` (`../cronmanager`). Success: pointing `svcman` at a host
finds out why a service will not start, and fixes it, faster than raw
`systemctl`/`journalctl`.

## Scope

v1 (0.1.0):

* interactive TUI for one host (`svcman [options] [user@]host`, `svcman --local`)
* audit mode (`svcman --audit [options] [user@]host`)
* system `.service` and `.timer` units only

Out of v1 (listed in `PENDING.md`): multi-host mode, `systemctl --user` units,
socket/path/mount units, full-unit override (`edit --full`).

## Architecture

Standalone Go module `github.com/kpatr1981/svcman` in `personal/tools/svcmanager`,
single static binary, no dependencies on the target beyond POSIX `sh` and systemd.

Copied from cronman and adapted (renamed identifiers, `svcman` prefixes); cronman is
not changed:

* `runner.go` — `Runner` interface, `sshRunner` (system `ssh`, ControlMaster with
  `ControlPath=/tmp/svcman-<rand>-%C`), `localRunner`, `RunScript`, `heredoc`, `shq`
* `term.go`, `term_unix.go`, `term_windows.go` — colours, raw keys, screen, line
  editor, `readPassword`
* keybar, diff and backup helpers

New files, one responsibility each:

| file | responsibility |
|---|---|
| `main.go` | flags (`-p -i -J -o --local --audit -v --fix --no-color --version`), dispatch |
| `units.go` | load script; parse `list-units`, `list-unit-files`, `systemctl show` blocks into `Unit` structs; timer↔service links |
| `probe.go` | remote sh script that collects file/user facts for one or many units; parser for its output |
| `checks.go` | pure functions: `Unit` + probe facts → `[]Issue` (severity, text, `[]Fix`); no I/O |
| `dropin.go` | read/parse/render/edit `/etc/systemd/system/<unit>.d/svcman.conf`; diff; backup; atomic write; daemon-reload with rollback |
| `journal.go` | last N journal lines for a unit, follow mode; `sudo -n` fallback |
| `actions.go` | start/stop/restart/enable/disable/reset-failed/daemon-reload with privilege handling |
| `tui.go` | list, details, journal view, drop-in editor, keys |
| `audit.go` | `--audit` report, `--fix`, exit status |

### Loading a host

One SSH round trip running one sh script that:

1. verifies `systemctl` exists (otherwise prints a marker; svcman exits 2 with
   "no systemd on <host>")
2. prints `systemctl list-units --all --type=service,timer --plain --no-legend`
3. prints `systemctl list-unit-files --type=service,timer --no-legend`
4. prints one `systemctl show` for all those units with
   `-p Id,Description,LoadState,ActiveState,SubState,Result,UnitFileState,FragmentPath,DropInPaths,ExecStart,ExecStartPre,ExecStop,User,Group,DynamicUser,WorkingDirectory,EnvironmentFiles,NRestarts,ExecMainStatus,StateChangeTimestamp,Triggers,TriggeredBy,NextElapseUSecRealtime,LastTriggerUSec,NeedDaemonReload,Restart`
   (blocks separated by blank lines, `key=value`)

Sections are separated by unique markers. Unparseable entries become a `Unit` with
state `?`, never a crash.

### Probe

`probe.go` builds a sh script for a set of units. For each ExecStart*/ExecStop
binary (leading `-@:+!` prefixes stripped), `User=`, `Group=`, `WorkingDirectory`,
`EnvironmentFile` (leading `-` = optional), unit file and drop-in paths it reports:
exists, type, mode, owner, executable/readable as the unit's user (`sudo -n -u`
or `runuser` when root; otherwise checked by mode bits and marked approximate),
`#!` interpreter, CRLF, `noexec` mount. It also runs `systemd-analyze verify` per
unit file when available. Output is line-based `unit<TAB>kind<TAB>path<TAB>facts`.

Interactive mode probes, before the screen opens ("Checking N units…"), the same
units audit checks (enabled, failed, timer-linked); other units are probed when
their details screen opens, and the CHECK column shows `·` until then. `R`
re-probes everything that was probed.

### Privileges

* read: unprivileged (`systemctl show` works as any user); journal falls back to
  `sudo -n` when the login user cannot read it
* write (actions, drop-ins, daemon-reload, fixes): run directly when logged in as
  root; `sudo -n` when passwordless sudo works (detected once at connect);
  otherwise svcman asks for the sudo password once per session (hidden, read
  locally; the screen is suspended while it asks), checks it with
  `sudo -k -S -p '' true`, and runs root commands as
  `sudo -k -S -p '' sh -c '<script>'` with the password on stdin, so their
  output can be shown. An empty password means "no root": actions fail with a
  message, reads continue unprivileged.
* audit: as cronman — root, `sudo -n`, or ask the sudo password once and use
  `sudo -k -S -p ''` (empty password = continue unprivileged with a warning)

### Platforms

Client: macOS, Linux, Windows (as cronman). Target: Linux with systemd ≥ 230.
`--local` is Linux-only (refused elsewhere with a message).

## Interactive mode

```
web1  ·  142 services, 18 timers  ·  filter: all          3 failed
 UNIT                         ACTIVE      ENABLED    CHECK  RESTARTS  SINCE
 nginx.service                running     enabled    ok            0   3d
 myapp.service                failed      enabled    ✗ 2           5   12m
 backup.timer                 waiting     enabled    ok            -   next 02:00
   └ backup.service           dead        static     ok            -   last ✓ 22h
```

* CHECK: `ok`, `✗ N` (failing issues), `! N` (warnings only), `…` (not probed yet)
* a timer's triggered service is indented under it (and not listed again at top
  level); SINCE shows next trigger for timers, last result (`✓`/`✗` from `Result`)
  and age for their services
* sorted: failed first, then by name

Keys (always shown in a keybar at the bottom):

| key | action |
|---|---|
| `↑ ↓` / `j k`, `/` | move; incremental search by name |
| `f` | filter cycle: all → failed → running → enabled → timers |
| `s` / `S` / `R` | start / stop / restart (asks first) |
| `space` / `t` | enable / disable (asks first) |
| `l` | journal: last 200 lines; `F` follow live; `q` back |
| `enter` | details: unit file, drop-ins, ExecStart breakdown, issues, numbered fixes |
| `1-9` (details) | apply fix N (asks first) |
| `e` | edit the svcman drop-in for this unit |
| `w` | save drop-in edits |
| `c` | `systemctl cat` view |
| `r` / `?` / `q` | reload / help / quit (quit asks when drop-in edits are unsaved) |

Actions (`s S R space t`, fixes) apply immediately after confirmation. After an
action the unit is re-read; if it is `failed`, the last 10 journal lines are shown.

### Drop-in editing

* edits only `/etc/systemd/system/<unit>.d/svcman.conf`; vendor files and other
  drop-ins are never written (other drop-ins are shown read-only in details/`c`)
* editor: list of `Section/Key=value` lines; add, change, delete; the section
  defaults to `[Service]` for services and `[Timer]` for timers
* keys are validated against the property names `systemctl show` returned for the
  unit; unknown keys get a warning, not a refusal
* `ExecStart`, `ExecStartPre`, `ExecStartPost`, `ExecStop`, `ExecStopPost` replace
  the vendor value: each is written as an empty `Key=` reset line followed by the new
  value. Every other key is written as is (systemd appends list keys such as
  `Environment=`, which is what adding a variable needs)
* removing every key deletes the file (and the `.d` directory if empty)
* edits are pending until `w`

### Save (`w`)

1. show a unified diff of every pending drop-in change; ask once
2. for each file: compare the remote file's current content with what was loaded;
   if changed meanwhile, skip that unit and keep the edits (message)
3. back up the old file (if any) to `~/.svcman_backups/<unit>-<timestamp>.conf` on
   the host (login user's home)
4. write atomically (temp file in the same directory + `mv`), mode 0644 root:root
5. `systemctl daemon-reload`; on failure restore the backups, daemon-reload again,
   report
6. offer to restart the edited units that are running

## Checks

| check | severity | fix |
|---|---|---|
| Exec* binary missing / not executable as `User=` | fail | `chmod u+x` / `g+x` / `o+x` (least invasive, as cronman) |
| `#!` interpreter missing | fail | — |
| script has CRLF line endings | fail | `sed -i 's/\r$//' <file>` |
| binary on a `noexec` mount | fail | — |
| `User=` / `Group=` does not exist (not `DynamicUser=yes`) | fail | — |
| `WorkingDirectory` / required `EnvironmentFile` missing or unreadable | fail | `chmod`/`chown` |
| unit `failed` (`Result`: exit-code, signal, core-dump, timeout, start-limit-hit, …) with last journal lines | fail | `reset-failed` + `restart` |
| restart loop: `NRestarts` ≥ 5 | warn | — |
| enabled, not active, not failed, not triggered by a timer | warn | `start` |
| `systemd-analyze verify` output | warn | — |
| unit file / drop-in group- or world-writable, or not owned by root | fail | `chmod go-w` / `chown root:root` |
| `/etc/systemd/system` unit shadowing a `/usr/lib` or `/lib` unit of the same name | info | — |
| timer whose `Triggers` unit does not exist | fail | — |
| timer whose service's last `Result` is not success | warn | — |
| active timer whose next elapse (`NextElapseUSecRealtime`) is more than 5 minutes in the past (overdue) | warn | `restart` the timer |
| `NeedDaemonReload=yes` | warn | `daemon-reload` |
| `LoadState` `not-found`/`bad-setting`/`error` | fail | — |

Fixes run with the privilege handling above and always ask first.

## Audit mode

`svcman --audit [options] [user@]host`

* host lines: `systemctl is-system-running` (warn when degraded/maintenance),
  number of failed units, masked units still wanted by an enabled unit
* per unit (only units with issues; `-v` lists every probed unit): issues with
  severity, fixes numbered
* `--fix`: offer each fix in turn (y/n/q)
* exit status: 0 no failing units, 1 failing units, 2 error

## Error handling

* ssh failure (exit 255), no systemd, unreadable output → one-line error, exit 2
* action failure in the TUI → status line shows systemctl's stderr; TUI keeps running
* drop-in write/daemon-reload failure → rollback as above, message
* a unit that cannot be parsed → shown as `?`

## Testing

* unit tests: `show` block parser, list parsers, timer linking, every check
  (table-driven over `Unit` + fake probe facts), drop-in parse/render/edit
  (incl. `ExecStart=` reset), diff, probe output parser
* integration: `test/lima/provision-svc.sh` plants faulty units on the existing
  Lima VM `cm-target` (missing binary, non-existent user, CRLF script, restart
  loop, timer with missing service, world-writable unit file, failed unit,
  `NeedDaemonReload`); `make itest` runs `svcman --audit` against it and asserts
  each planted fault is reported and the exit status is 1
* manual TUI check on `cm-target`: list, filter, journal, start/stop, drop-in edit
  and save with rollback

## Release

`Makefile` with `build`, `test`, `itest`, `release` (6 platforms, `SHA256SUMS`),
version 0.1.0, README in cronman's style, `PENDING.md` with the v1 exclusions.
