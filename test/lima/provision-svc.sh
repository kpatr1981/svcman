#!/bin/sh
# Plant known-broken systemd units for svcman tests. Run as root. Arg: debian|rocky.
set -e
fam=$1
case $fam in debian) SUDOGRP=sudo; VENDOR=/usr/lib/systemd/system ;; rocky) SUDOGRP=wheel; VENDOR=/usr/lib/systemd/system ;; esac
command -v sudo >/dev/null || { [ $fam = debian ] && apt-get install -y -qq sudo >/dev/null || dnf -y -q install sudo; }

KEY=$(cat /tmp/lima.pub)
mk() {
  id "$1" >/dev/null 2>&1 || useradd -m -s /bin/bash "$1"
  echo "$1:pw-$1" | chpasswd
  h=$(eval echo ~"$1"); mkdir -p "$h/.ssh"; echo "$KEY" > "$h/.ssh/authorized_keys"
  chown -R "$1:" "$h/.ssh"; chmod 700 "$h/.ssh"; chmod 600 "$h/.ssh/authorized_keys"
}
mk alice; mk bob
echo 'bob ALL=(ALL) ALL' > /etc/sudoers.d/bob; chmod 440 /etc/sudoers.d/bob

S=/etc/systemd/system
mkdir -p /opt/svct
printf '#!/bin/sh\nexec sleep 100000\n' > /opt/svct/good.sh; chmod 755 /opt/svct/good.sh
printf '#!/bin/sh\r\nexec sleep 100000\r\n' > /opt/svct/crlf.sh; chmod 755 /opt/svct/crlf.sh
printf '#!/bin/sh\nexec sleep 100000\n' > /opt/svct/noexec.sh; chmod 644 /opt/svct/noexec.sh

unit() { # name, body
  printf '[Unit]\nDescription=svcman test %s\n\n%s\n' "$1" "$2" > "$S/$1"
  chmod 644 "$S/$1"
}
svc() { unit "$1" "[Service]
$2

[Install]
WantedBy=multi-user.target"; }

svc svct-good.service      'Type=oneshot
ExecStart=/bin/true'
unit svct-good.timer       '[Timer]
OnCalendar=*-*-* 03:00:00
Unit=svct-good.service

[Install]
WantedBy=timers.target'
svc svct-missing.service   'ExecStart=/opt/svct/nope'
svc svct-baduser.service   'User=nosuchuser
ExecStart=/opt/svct/good.sh'
svc svct-crlf.service      'ExecStart=/opt/svct/crlf.sh'
svc svct-noexec.service    'ExecStart=/opt/svct/noexec.sh'
svc svct-loop.service      'ExecStart=/bin/false
Restart=always
RestartSec=1
StartLimitIntervalSec=0'
unit svct-orphan.timer     '[Timer]
OnCalendar=hourly
Unit=svct-ghost.service

[Install]
WantedBy=timers.target'
svc svct-perm.service      'ExecStart=/opt/svct/good.sh'
chmod 666 $S/svct-perm.service
svc svct-reload.service    'ExecStart=/opt/svct/good.sh'
svc svct-badtimer.service  'Type=oneshot
ExecStart=/bin/false'
unit svct-badtimer.timer   '[Timer]
OnCalendar=*-*-* 04:00:00

[Install]
WantedBy=timers.target'
printf '[Unit]\nDescription=vendor svct-shadow\n[Service]\nExecStart=/opt/svct/good.sh\n' > $VENDOR/svct-shadow.service
svc svct-shadow.service    'ExecStart=/opt/svct/good.sh'

systemctl daemon-reload
for u in svct-good.timer svct-missing.service svct-baduser.service svct-crlf.service svct-noexec.service \
         svct-loop.service svct-orphan.timer svct-perm.service svct-reload.service svct-badtimer.timer; do
  systemctl enable "$u" >/dev/null 2>&1 || true
done
systemctl start svct-good.timer svct-badtimer.timer svct-loop.service svct-reload.service 2>/dev/null || true
systemctl start svct-missing.service svct-baduser.service svct-crlf.service svct-noexec.service 2>/dev/null || true
systemctl start svct-badtimer.service 2>/dev/null || true
sleep 8   # let svct-loop restart a few times
echo '# edited after load' >> $S/svct-reload.service   # NeedDaemonReload=yes
echo "provisioned $fam"
