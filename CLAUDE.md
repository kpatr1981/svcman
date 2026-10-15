# svcman

Go TUI/audit tool for systemd over SSH, built in the same fashion as `../cronmanager`
(runner.go and term*.go were copied from there; fixes to them belong in both).
Spec: docs/superpowers/specs/2026-10-15-svcman-design.md.
Tests: `make test`; end to end `sh test/lima/create.sh && make itest` (Lima VMs svc-debian, svc-rocky).
