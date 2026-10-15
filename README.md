# svcman

Manage and audit systemd services and timers on remote hosts over SSH.

```
svcman [options] [user@]host           # interactive manager for the host's services and timers
svcman --audit [options] [user@]host   # check every enabled, failed and timer unit
svcman --local [--audit]               # this machine instead of SSH (Linux)
```

## Interactive mode

`svcman admin@web1` lists every service and timer on the host. Failed units come first,
and each timer has the service it triggers indented under it:

```
 svcman  admin@web1  142 services, 10 timers  ·  filter: all  3 failed
 UNIT                         ACTIVE       ENABLED   CHECK  RESTARTS  SINCE
 myapp.service                failed       enabled   ✗ 2           5  12m
 nginx.service                running      enabled   ok            0  3d
 backup.timer                 waiting      enabled   ok            -  next 10-16 02:00
   └ backup.service           dead         static    ok            0  last ✓ 22h
```

| key | action |
|---|---|
| `↑ ↓` / `j k`, `pgup pgdn`, `g G` | move |
| `/` | search unit names (empty clears) |
| `f` | filter: all → failed → running → enabled → timers |
| `s` / `S` / `R` | start / stop / restart (asks first) |
| `space` / `t` | enable / disable (asks first) |
| `l` | journal: the last 200 lines; `F` follows it live, `ctrl-c` returns |
| `enter` | details: unit file, drop-ins, commands, user, timer times, problems and numbered fixes |
| `1-9` (details) | apply a proposed fix (asks first, runs as root) |
| `e` | edit svcman's drop-in for the unit (see below) |
| `w` | save drop-in edits |
| `c` | `systemctl cat`: the unit as systemd sees it, drop-ins included |
| `r` / `?` / `q` | reload from the host / help / quit (asks when drop-in edits are unsaved) |

The key bindings are always shown at the bottom of the screen.

Start, stop, restart, enable, disable and fixes run as soon as you confirm them. After an
action the unit is read again; if it ended up `failed`, its last 10 journal lines are shown.

| CHECK | meaning |
|---|---|
| `ok` | no problems found |
| `✗ N` | N failing checks |
| `! N` | N warnings, nothing failing |
| `·` | files not checked yet (opening the details checks them) |

Before the list opens, svcman checks the same units audit mode does: enabled, failed,
timers and the services they trigger. Other units are checked when you open their details.
SINCE is how long the unit has been in its state; for a timer it is the next run, and for
a timer's service it is the last result (`✓`/`✗`) and when that was.

Units that are only referenced and do not exist (`NetworkManager.service` on a host without
it) are not listed, except under a timer that triggers them.

### Drop-ins

`e` edits `/etc/systemd/system/<unit>.d/svcman.conf`, a drop-in that only svcman writes.
Vendor unit files and other drop-ins (for example `override.conf` from `systemctl edit`)
are never changed; they are listed read-only and `c` shows them.

* `a` adds a setting as `Key=value` (in `[Service]`, or `[Timer]` for a timer) or
  `Section/Key=value`, `enter` changes one, `D` deletes one.
* A key that systemd does not report for the unit is marked `(unknown to systemd?)`, so
  typos such as `Restrat=` stand out.
* `ExecStart=`, `ExecStartPre=`, `ExecStartPost=`, `ExecReload=`, `ExecStop=` and
  `ExecStopPost=` replace the unit's commands: svcman writes the empty `ExecStart=` reset
  line before the new value. Every other key is written as is (so `Environment=FOO=bar`
  adds a variable).
* `w` shows the change, asks once, then for each drop-in: refuses if the file changed on
  the host since svcman read it (your edits are kept), backs up the old file to
  `~/.svcman_backups/` on the host, writes the new one atomically (mode 0644, root),
  runs `systemctl daemon-reload` (and restores the backup if that fails), and offers to
  restart the edited units that are running.
* Deleting every setting deletes the file. Deleting `svcman.conf` by hand undoes everything
  svcman changed.

### What is checked

As the user the unit runs as (`User=`, else root), from systemd's own view of the unit:

* the unit loads (`not-found`, `bad-setting`, `error`), is not failed (with the result
  and last log lines), is not in a restart loop (5 or more restarts)
* an enabled service that ran and was stopped (enabled units that never started because a
  start condition was not met, or their target was never reached, are normal)
* `systemctl daemon-reload` is needed (unit file changed on disk)
* every `ExecStart=`/`ExecStartPre=`/`ExecStop=`… command exists, is executable (and
  readable, for scripts) by the unit's user; bare names (`networkctl`) are looked up in
  systemd's search path
* `#!` interpreter exists; `#!/usr/bin/env node` finds `node` in systemd's `PATH`
* Windows CRLF line endings (`/bin/sh^M: bad interpreter`), `noexec` mounts
* `User=` and `Group=` exist; `WorkingDirectory=` can be entered and `EnvironmentFile=`
  read (a leading `-` makes them optional)
* unit file and drop-ins are owned by root and not writable by group or others
* a unit in `/etc/systemd/system` that hides a vendor unit of the same name
* timers: the unit they trigger exists, its last run succeeded, and the timer is not overdue
* `systemd-analyze verify` messages about the unit

Fixes are the least invasive change: `chmod u+x` (or `o+x`, by who needs access),
`chmod go-w` / `chown root:root` on unit files, stripping CRs, `reset-failed` and
restart, `daemon-reload`.

## Audit mode

`svcman --audit admin@web1` checks every enabled unit, every failed unit, every timer and
the services timers trigger, and units that need a daemon-reload. It also reports:

* the system state (`degraded`, `maintenance`) and the list of failed units
* masked units that an enabled unit still wants or requires

Only problems are printed (`-v` checks and lists every unit). `--fix` offers each fix in
turn (`y`/`n`/`q`). Exit status: 0 = no failing units, 1 = failing units, 2 = error.

## Privileges

Reading needs no privileges: `systemctl show` works for any user. Everything that changes
the host, and the exact per-user file checks, need root. svcman uses, in this order:

* root, when you log in as root
* `sudo -n`, when the login user has passwordless sudo
* your sudo password, asked once per run on your terminal (never shown, never sent
  anywhere but `sudo -S` on the host). Press enter on an empty prompt to continue without
  root.

Without root svcman still lists, checks (from permission bits, marked as such) and shows
the journal as far as the login user may read it; actions say that they need root. Without
a terminal (cron, CI) it never waits for a password.

## Authentication

svcman does not implement SSH itself. It runs your system `ssh`, so every login method
`ssh` supports works:

* **SSH keys:** default keys, `-i ~/.ssh/id_ed25519`, ssh-agent, hardware keys
* **passwords and key passphrases:** prompted by `ssh` in your terminal
* **`~/.ssh/config`:** host aliases, `User`, `Port`, `ProxyJump`, `IdentityFile` all apply
  (`svcman web1` works if `web1` is a Host entry)
* `-p PORT`, `-i KEY`, `-J JUMPHOST` and `-o Option=value` are passed straight to `ssh`

On macOS/Linux the connection is opened once and reused (`ControlMaster`), so a password
or passphrase is asked for only once per run.

## Platforms

The client runs on **macOS, Linux and Windows** (uses the system `ssh`; Windows 10+
ships OpenSSH). The target must be Linux with systemd 230 or newer; only POSIX `sh` and
standard tools are needed there — nothing is installed. Tested on Debian 12 and Rocky 9.

On Windows, OpenSSH has no connection sharing, so use key-based auth (or ssh-agent) to
avoid a password prompt per operation.

## Install

Download a prebuilt binary from the
[Releases page](https://github.com/kpatr1981/svcman/releases/latest).
They are static, single-file executables with no dependencies.

| OS | CPU | file |
|---|---|---|
| Linux | x86-64 | `svcman-linux-amd64` |
| Linux | ARM64 (Raspberry Pi 4/5, Graviton) | `svcman-linux-arm64` |
| macOS | Intel | `svcman-darwin-amd64` |
| macOS | Apple Silicon | `svcman-darwin-arm64` |
| Windows | x86-64 | `svcman-windows-amd64.exe` |
| Windows | ARM64 | `svcman-windows-arm64.exe` |

```sh
curl -LO https://github.com/kpatr1981/svcman/releases/latest/download/svcman-linux-amd64
chmod +x svcman-linux-amd64
sudo mv svcman-linux-amd64 /usr/local/bin/svcman
svcman --version
```

Verify the download against `SHA256SUMS` from the same release:
`shasum -a 256 -c SHA256SUMS --ignore-missing`.

## Build from source

You need [Go](https://go.dev/dl/) 1.26 or newer. There is no cgo.

```sh
git clone https://github.com/kpatr1981/svcman.git
cd svcman
make build                     # ./svcman
make test                      # go vet + unit tests
make release                   # dist/ binaries for linux, macOS, windows (amd64/arm64) + SHA256SUMS
```

End-to-end tests run against two [Lima](https://lima-vm.io) VMs with deliberately broken
units:

```sh
sh test/lima/create.sh         # creates and provisions svc-debian and svc-rocky
make itest                     # svcman --audit must report every planted fault
```

## Author

Konstantinos Patronas — <kpatronas@gmail.com>
