#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-only
# Copyright (C) 2026 The Lemongrass Authors

# rpm passes 0 on a real removal and 1 or more on an upgrade. The per-user setup lives in each home, out of reach of
# a root scriptlet, so a removal only reminds; the in-app Uninstall runs lgrassconf uninstall before this.
if [ "$1" = 0 ]; then
  echo "Lemongrass: as each user who ran it, run 'lgrassconf uninstall' to remove its keeper, hooks and commands." >&2
fi
exit 0
