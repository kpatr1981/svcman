#!/bin/sh
# Create and provision the svcman test VMs (idempotent). Usage: sh test/lima/create.sh [debian|rocky]...
set -e
cd "$(dirname "$0")"
for fam in ${*:-debian rocky}; do
  vm=svc-$fam
  case $fam in debian) tpl=template:debian-12 ;; rocky) tpl=template:rocky-9 ;; *) echo "unknown $fam"; exit 2 ;; esac
  if ! limactl list -q 2>/dev/null | grep -qx "$vm"; then
    limactl create --tty=false --name="$vm" --cpus=1 --memory=1 --disk=12 "$tpl"
  fi
  limactl start "$vm" >/dev/null 2>&1 || true
  limactl copy provision-svc.sh "$vm":/tmp/provision-svc.sh
  limactl copy ~/.lima/_config/user.pub "$vm":/tmp/lima.pub
  limactl shell "$vm" sudo sh /tmp/provision-svc.sh "$fam"
done
