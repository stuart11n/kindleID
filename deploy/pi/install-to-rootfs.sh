#!/bin/bash
# Install kindle-shazam onto a mounted Raspberry Pi rootfs (+ optional bootfs).
# Usage: install-to-rootfs.sh <rootfs> [bootfs] [binary]
set -euo pipefail

ROOTFS=${1:?rootfs mount point}
BOOTFS=${2:-}
BIN=${3:-kindle-shazam-linux-armv6}
USER_NAME=${PI_USER:-stuart}
PUBKEY=${PI_PUBKEY:-}

if [[ ! -f "$BIN" ]]; then
  echo "missing binary: $BIN" >&2
  exit 1
fi
if [[ ! -d "$ROOTFS/etc" ]]; then
  echo "not a rootfs: $ROOTFS" >&2
  exit 1
fi

install -d "$ROOTFS/usr/local/bin"
install -d "$ROOTFS/var/lib/kindle-shazam"
install -d "$ROOTFS/etc/systemd/system"
install -m 755 "$BIN" "$ROOTFS/usr/local/bin/kindle-shazam"
install -m 644 "$(dirname "$0")/kindle-shazam.service" "$ROOTFS/etc/systemd/system/kindle-shazam.service"

# Enable service (systemd wants relative symlink under wants/)
mkdir -p "$ROOTFS/etc/systemd/system/multi-user.target.wants"
ln -sfn /etc/systemd/system/kindle-shazam.service \
  "$ROOTFS/etc/systemd/system/multi-user.target.wants/kindle-shazam.service"

# Ensure ALSA utils exist note — package install needs online apt on first boot.
# Drop a oneshot that installs arecord if missing.
cat >"$ROOTFS/usr/local/bin/kindle-shazam-deps.sh" <<'EOF'
#!/bin/bash
set -e
if ! command -v arecord >/dev/null 2>&1; then
  export DEBIAN_FRONTEND=noninteractive
  apt-get update -qq
  apt-get install -y -qq alsa-utils
fi
EOF
chmod 755 "$ROOTFS/usr/local/bin/kindle-shazam-deps.sh"

cat >"$ROOTFS/etc/systemd/system/kindle-shazam-deps.service" <<'EOF'
[Unit]
Description=Ensure ALSA tools for kindle-shazam
Before=kindle-shazam.service
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/kindle-shazam-deps.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
EOF
mkdir -p "$ROOTFS/etc/systemd/system/multi-user.target.wants"
ln -sfn /etc/systemd/system/kindle-shazam-deps.service \
  "$ROOTFS/etc/systemd/system/multi-user.target.wants/kindle-shazam-deps.service"

# Create user + SSH if bootfs provided (first-boot) or inject into rootfs.
if [[ -n "$BOOTFS" && -d "$BOOTFS" ]]; then
  : >"$BOOTFS/ssh"
  if [[ -n "$PUBKEY" && -f "$PUBKEY" ]]; then
    # cloud-init style: also place pubkey for first boot via firstrun if needed
    install -d "$BOOTFS/kindle-shazam"
    cp "$PUBKEY" "$BOOTFS/kindle-shazam/authorized_keys"
  fi
fi

# Always ensure SSH enabled in rootfs and user exists with authorized_keys.
if [[ ! -f "$ROOTFS/etc/ssh/sshd_config" ]]; then
  echo "warning: no sshd_config on rootfs yet" >&2
fi
# Enable ssh service
mkdir -p "$ROOTFS/etc/systemd/system/multi-user.target.wants"
ln -sfn /lib/systemd/system/ssh.service \
  "$ROOTFS/etc/systemd/system/multi-user.target.wants/ssh.service" 2>/dev/null || \
ln -sfn /usr/lib/systemd/system/ssh.service \
  "$ROOTFS/etc/systemd/system/multi-user.target.wants/ssh.service" 2>/dev/null || true
# Also the raspbian alias
ln -sfn /lib/systemd/system/ssh.service \
  "$ROOTFS/etc/systemd/system/sshd.service" 2>/dev/null || true
touch "$ROOTFS/boot/ssh" 2>/dev/null || true
touch "$ROOTFS/boot/firmware/ssh" 2>/dev/null || true

if ! grep -q "^${USER_NAME}:" "$ROOTFS/etc/passwd" 2>/dev/null; then
  # uid 1000 typical
  echo "${USER_NAME}:x:1000:1000::/home/${USER_NAME}:/bin/bash" >>"$ROOTFS/etc/passwd"
  echo "${USER_NAME}:*:19000:0:99999:7:::" >>"$ROOTFS/etc/shadow"
  echo "${USER_NAME}:x:1000:" >>"$ROOTFS/etc/group"
  # passwordless sudo like pi user
  if [[ -d "$ROOTFS/etc/sudoers.d" ]]; then
    echo "${USER_NAME} ALL=(ALL) NOPASSWD:ALL" >"$ROOTFS/etc/sudoers.d/010_${USER_NAME}-nopasswd"
    chmod 440 "$ROOTFS/etc/sudoers.d/010_${USER_NAME}-nopasswd"
  fi
fi
install -d -m 700 "$ROOTFS/home/${USER_NAME}"
install -d -m 700 "$ROOTFS/home/${USER_NAME}/.ssh"
if [[ -n "$PUBKEY" && -f "$PUBKEY" ]]; then
  install -m 600 "$PUBKEY" "$ROOTFS/home/${USER_NAME}/.ssh/authorized_keys"
fi
# Ownership will be wrong from Windows/WSL root copy — fix numeric
chown -R 1000:1000 "$ROOTFS/home/${USER_NAME}" || true

# audio group membership
if grep -q '^audio:' "$ROOTFS/etc/group"; then
  if ! grep -q "^audio:.*${USER_NAME}" "$ROOTFS/etc/group"; then
    sed -i "s/^audio:\([^:]*\):\([^:]*\):\(.*\)/audio:\1:\2:\3,${USER_NAME}/" "$ROOTFS/etc/group"
    sed -i "s/:,/,/; s/,$//" "$ROOTFS/etc/group" || true
  fi
fi

echo "installed kindle-shazam into $ROOTFS"
