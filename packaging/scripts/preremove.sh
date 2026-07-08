#!/bin/sh
# Copyright (c) 2026 Chatic Contributors
# Licensed under the Apache License, Version 2.0.
set -e

# Stop and disable the service before removing the binary. Data in
# /var/lib/chatic is deliberately PRESERVED (we don't delete user data on
# removal — remove it manually if you want a clean uninstall).
if command -v systemctl >/dev/null 2>&1; then
    systemctl stop chatic.service || true
    systemctl disable chatic.service || true
fi

exit 0
