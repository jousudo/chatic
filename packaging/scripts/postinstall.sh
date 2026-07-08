#!/bin/sh
# Copyright (c) 2026 Chatic Contributors
# Licensed under the Apache License, Version 2.0.
set -e

DATA_DIR=/var/lib/chatic
ENV_FILE="$DATA_DIR/.env"
ENV_SAMPLE=/usr/share/chatic/chatic.env.example

# 1. Create a no-login system user (idempotent; covers both deb and rpm).
if ! id chatic >/dev/null 2>&1; then
    if command -v useradd >/dev/null 2>&1; then
        useradd --system --home-dir "$DATA_DIR" --shell /usr/sbin/nologin chatic || true
    elif command -v adduser >/dev/null 2>&1; then
        adduser --system --home "$DATA_DIR" --no-create-home chatic || true
    fi
fi

# 2. Data directory + default .env (does not overwrite an existing config).
mkdir -p "$DATA_DIR/storage"
if [ ! -f "$ENV_FILE" ] && [ -f "$ENV_SAMPLE" ]; then
    cp "$ENV_SAMPLE" "$ENV_FILE"
fi
chown -R chatic:chatic "$DATA_DIR" 2>/dev/null || true
chmod 600 "$ENV_FILE" 2>/dev/null || true

# 3. systemd service: reload, enable and start.
if command -v systemctl >/dev/null 2>&1; then
    systemctl daemon-reload || true
    systemctl enable chatic.service || true
    systemctl restart chatic.service || true
fi

echo "============================================================"
echo " Chatic is installed and running as a service."
echo "------------------------------------------------------------"
echo " 1) (Optional) Edit $ENV_FILE to set INITIAL_ADMIN_NUMBER,"
echo "    then: sudo systemctl restart chatic"
echo " 2) Open the panel:  http://YOUR_SERVER:3030/admin"
echo "    - create the panel password (first access)"
echo "    - scan the QR Code to pair WhatsApp"
echo " 3) Audio (voice/TTS) is OPTIONAL and needs FFmpeg. It is a recommended"
echo "    dependency, so apt/dnf usually install it automatically. If not:"
echo "    Debian/Ubuntu: sudo apt install ffmpeg | Fedora: sudo dnf install ffmpeg"
echo "============================================================"

exit 0
