#!/bin/sh
set -e

# Ensure the data directory exists and is writable by the unprivileged user.
# This runs as root so it works whether the mounted volume is owned by root
# (bind mounts) or already user-owned (named volumes).
DATA_DIR="${DATA_DIR:-/data}"
mkdir -p "$DATA_DIR"
chown -R filez:filez "$DATA_DIR" 2>/dev/null || true

# Drop privileges and run the server.
exec su-exec filez "$@"
