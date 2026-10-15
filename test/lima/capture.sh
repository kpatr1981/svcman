#!/bin/sh
# Capture real systemctl show output into testdata/ for parser tests.
set -e
cd "$(dirname "$0")/../.."
PROPS=Id,Description,Type,LoadState,ActiveState,SubState,Result,UnitFileState,FragmentPath,DropInPaths,ExecStartPre,ExecStart,ExecStartPost,ExecReload,ExecStop,ExecStopPost,User,Group,DynamicUser,WorkingDirectory,EnvironmentFiles,NRestarts,Restart,Triggers,TriggeredBy,Wants,Requires,StateChangeTimestamp,InactiveExitTimestamp,NextElapseUSecRealtime,LastTriggerUSec,NeedDaemonReload
mkdir -p testdata
for fam in debian rocky; do
  limactl shell svc-$fam sh -c "LC_ALL=C TZ=UTC systemctl show --no-pager -p $PROPS \
    ssh.service sshd.service cron.service crond.service systemd-journald.service \
    \$(systemctl list-units --all --plain --no-legend '*@*.service' | awk '{print \$1}' | head -3) \
    \$(systemctl list-unit-files --no-legend 'svct-*' | awk '{print \$1}')" > testdata/show-$fam.txt
  wc -l testdata/show-$fam.txt
done
