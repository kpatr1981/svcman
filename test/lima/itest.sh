#!/bin/sh
# End-to-end: svcman --audit against the provisioned VMs must report every planted fault.
set -u
cd "$(dirname "$0")/../.."
fail=0
for vm in ${VMS:-svc-debian svc-rocky}; do
  user=$(limactl shell "$vm" id -un)
  out=$(./svcman --no-color --audit $(sh test/lima/ssh-args.sh "$vm") "$user@127.0.0.1" 2>&1 </dev/null); rc=$?
  echo "== $vm (exit $rc)"
  [ $rc -eq 1 ] || { echo "FAIL: exit $rc, want 1"; fail=1; }
  for want in \
    "ExecStart /opt/svct/nope does not exist" \
    "user nosuchuser does not exist" \
    "/opt/svct/crlf.sh has Windows (CRLF) line endings" \
    "/opt/svct/noexec.sh is not executable by root" \
    "restarted" \
    "triggers svct-ghost.service, which does not exist" \
    "svct-perm.service is writable by group or others" \
    "daemon-reload" \
    "last run of svct-badtimer.service failed" \
    "overrides the vendor unit"; do
    printf '%s' "$out" | grep -qF -- "$want" || { echo "FAIL: missing: $want"; fail=1; }
  done
  printf '%s' "$out" | grep -q '^svct-good' && { echo "FAIL: svct-good reported"; fail=1; }
  # Non-root, no sudo: must still run, judged from permission bits.
  out2=$(./svcman --no-color --audit $(sh test/lima/ssh-args.sh "$vm") alice@127.0.0.1 2>&1 </dev/null); rc2=$?
  [ $rc2 -eq 1 ] || { echo "FAIL: alice exit $rc2"; echo "$out2" | tail -5; fail=1; }
  printf '%s' "$out2" | grep -qF "permission bits" || { echo "FAIL: alice run did not warn about approximate checks"; fail=1; }
done
[ $fail -eq 0 ] && echo "itest: all good"
exit $fail
