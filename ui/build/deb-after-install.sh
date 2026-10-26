#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Faizal Alviansyah

set -e
case "$(uname -m)" in
  aarch64) arch=arm64 ;;
  *) arch=amd64 ;;
esac
mkdir -p /usr/lib/systemd/user
systemctl --global disable --now lgconf.service || true
rm -f /usr/lib/systemd/user/lgconf.service
cat > /usr/lib/systemd/user/lgrassconf.service <<UNIT
[Unit]
Description=lemongrass agent config keeper

[Service]
Type=simple
ExecStart=/opt/Lemongrass/resources/bin/linux-$arch/lgrassconf run
Restart=on-failure
RestartSec=2

[Install]
WantedBy=default.target
UNIT
chmod 0755 /opt/Lemongrass/resources/bin/linux-$arch/lgrassconf || true
systemctl --global enable lgrassconf.service || true
