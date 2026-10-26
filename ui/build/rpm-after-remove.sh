#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 Faizal Alviansyah

if [ "$1" = "0" ]; then
  systemctl --global disable lgrassconf.service || true
  rm -f /usr/lib/systemd/user/lgrassconf.service
  rm -f /etc/systemd/user/default.target.wants/lgrassconf.service
fi
