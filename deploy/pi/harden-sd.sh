#!/bin/bash
# Cut SD-card writes on a headless Raspberry Pi running kindle-shazam.
# Run once as root:  sudo bash harden-sd.sh
set -euo pipefail

echo "=== disable swap ==="
if command -v dphys-swapfile >/dev/null 2>&1; then
  dphys-swapfile swapoff || true
  systemctl disable --now dphys-swapfile || true
fi
if [ -f /etc/dphys-swapfile ]; then
  sed -i 's/^CONF_SWAPSIZE=.*/CONF_SWAPSIZE=0/' /etc/dphys-swapfile
fi
swapoff -a 2>/dev/null || true
rm -f /var/swap

echo "=== disable background writers ==="
for u in apt-daily.timer apt-daily-upgrade.timer man-db.timer dpkg-db-backup.timer; do
  systemctl disable --now "$u" 2>/dev/null || true
  systemctl mask "$u" 2>/dev/null || true
done

echo "=== fstab commit=600 ==="
if grep -q 'commit=' /etc/fstab; then
  echo "commit already set"
else
  sed -i -E 's|(PARTUUID=[^ ]+\s+/\s+ext4\s+)([^ ]+)|\1\2,commit=600|' /etc/fstab
  if ! grep -E '/\s+ext4\s+.*commit=' /etc/fstab >/dev/null; then
    sed -i -E 's|(\s+/\s+ext4\s+)defaults,noatime|\1defaults,noatime,commit=600|' /etc/fstab
  fi
fi
mount -o remount,commit=600 / || true

echo "=== verify ==="
swapon --show || true
free -h | head -2
systemctl is-enabled apt-daily.timer apt-daily-upgrade.timer man-db.timer dpkg-db-backup.timer 2>&1 || true
grep -E 'ext4|/ ' /etc/fstab || true
findmnt -no OPTIONS /
echo HARDEN_OK
