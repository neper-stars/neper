#!/usr/bin/env bash
#
# Fly.io Infrastructure Teardown Script
# WARNING: This will DELETE all resources including the database!
#
# Usage: ./deploy/fly/teardown.sh
#
set -euo pipefail

APP_NAME="${NEPER_FLY_APP_NAME:-neper}"
DB_NAME="${NEPER_FLY_DB_NAME:-neper-db}"

log() { echo "==> $*"; }

echo "========================================"
echo "WARNING: This will permanently delete:"
echo "  - App: $APP_NAME"
echo "  - Database: $DB_NAME (ALL DATA LOST!)"
echo "  - Volumes and all stored files"
echo "========================================"
echo ""
read -p "Type 'yes' to confirm deletion: " confirm

if [[ "$confirm" != "yes" ]]; then
    echo "Aborted."
    exit 1
fi

log "Destroying app '$APP_NAME'..."
fly apps destroy "$APP_NAME" --yes 2>/dev/null || log "App not found or already destroyed"

log "Destroying database '$DB_NAME'..."
fly postgres destroy "$DB_NAME" --yes 2>/dev/null || log "Database not found or already destroyed"

log "Teardown complete."
