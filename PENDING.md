# svcman — pending

- [ ] Multi-host mode: the same unit on several hosts (`svcman h1,h2`), like cronman's
- [ ] `systemctl --user` units of the login user
- [ ] socket, path and mount units in the list and checks
- [ ] Full-unit override (`systemctl edit --full` equivalent)
- [x] Publish: github.com/kpatr1981/svcman, v0.1.0 release with binaries and installers (2026-10-15)
- [ ] Integration test on Ubuntu 24.04 and an old systemd (Debian 10, systemd 241)
- [ ] Run install.ps1 on a real Windows machine (PATH update, `svcman --version`); only tested under pwsh on macOS
- [x] Search for a service that sits under a timer selects the timer row, so `e` edits the timer's drop-in; select the matching row instead
- [x] Quote the unit name in saveScript's backup path with shq (safe today: unit names cannot contain shell-special characters)
- [x] Probe request lines are tab-separated: an Exec path with a tab or newline would be misread
- [x] Login-shell noise on stderr (e.g. bash setlocale warnings) ends up in error messages built from stderr
