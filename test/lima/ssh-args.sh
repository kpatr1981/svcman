#!/bin/sh
# Print svcman SSH options for a Lima VM: sh test/lima/ssh-args.sh svc-debian
port=$(limactl list "$1" --format '{{.SSHLocalPort}}')
echo "-p $port -i $HOME/.lima/_config/user -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"
