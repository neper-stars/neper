#!/usr/bin/env bash
#
# Fly.io Infrastructure Setup Script
# Run this once to create all Fly.io resources for neper
#
# Usage: ./deploy/fly/setup.sh
#
# Prerequisites:
#   - flyctl installed: curl -L https://fly.io/install.sh | sh
#   - Authenticated: fly auth login
#
set -euo pipefail

# =============================================================================
# Configuration - Edit these values
# =============================================================================
APP_NAME="${NEPER_FLY_APP_NAME:-neper}"
DB_NAME="${NEPER_FLY_DB_NAME:-neper-db}"
REGION="${NEPER_FLY_REGION:-cdg}"  # Paris, France
VOLUME_SIZE="${NEPER_FLY_VOLUME_SIZE:-1}"  # GB

# Admin user settings (for auto-creation on first start)
ADMIN_USERNAME="${NEPER_ADMIN_USERNAME:-admin}"
ADMIN_EMAIL="${NEPER_ADMIN_EMAIL:-admin@neper.local}"

# =============================================================================
# Helper functions
# =============================================================================
log() { echo "==> $*"; }
error() { echo "ERROR: $*" >&2; exit 1; }

check_flyctl() {
    if ! command -v flyctl &> /dev/null && ! command -v fly &> /dev/null; then
        error "flyctl not found. Install with: curl -L https://fly.io/install.sh | sh"
    fi
}

generate_nats_nkey() {
    # If provided via env, use it
    if [[ -n "${NEPER_NATS_NKEY:-}" ]]; then
        echo "$NEPER_NATS_NKEY"
        return
    fi

    # Try to use neper command to generate proper NATS nkey
    if command -v neper &> /dev/null; then
        neper generate-nkey --type user 2>/dev/null | grep -A1 "Seed" | tail -1
        return
    fi

    # Try via go run if in the repo
    if [[ -f "go.mod" ]] && command -v go &> /dev/null; then
        go run ./cmd/neper generate-nkey --type user 2>/dev/null | grep -A1 "Seed" | tail -1
        return
    fi

    error "Cannot generate NATS nkey. Either:
  1. Set NEPER_NATS_NKEY environment variable with a valid nkey seed (starts with SU)
  2. Build neper first: go build -o neper ./cmd/neper
  3. Generate manually: neper generate-nkey --type user"
}

app_exists() {
    flyctl apps list --json 2>/dev/null | grep -q "\"$1\"" 2>/dev/null
}

db_exists() {
    flyctl postgres list --json 2>/dev/null | grep -q "\"$1\"" 2>/dev/null
}

volume_exists() {
    flyctl volumes list --app "$APP_NAME" --json 2>/dev/null | grep -q "neper_data" 2>/dev/null
}

# =============================================================================
# Main setup
# =============================================================================
main() {
    log "Fly.io Setup for Neper"
    log "======================"
    echo "App Name:    $APP_NAME"
    echo "DB Name:     $DB_NAME"
    echo "Region:      $REGION"
    echo "Volume Size: ${VOLUME_SIZE}GB"
    echo ""

    check_flyctl

    # Step 1: Create the app
    if app_exists "$APP_NAME"; then
        log "App '$APP_NAME' already exists, skipping creation"
    else
        log "Creating app '$APP_NAME'..."
        flyctl apps create "$APP_NAME" --org personal
    fi

    # Step 2: Create Postgres database
    if db_exists "$DB_NAME"; then
        log "Database '$DB_NAME' already exists, skipping creation"
    else
        log "Creating Postgres database '$DB_NAME'..."
        flyctl postgres create \
            --name "$DB_NAME" \
            --region "$REGION" \
            --vm-size shared-cpu-1x \
            --initial-cluster-size 1 \
            --volume-size "$VOLUME_SIZE"
    fi

    # Step 3: Attach database to app
    log "Attaching database to app..."
    fly postgres attach "$DB_NAME" --app "$APP_NAME" 2>/dev/null || log "Database already attached or connection exists"

    # Step 4: Create persistent volume
    if volume_exists; then
        log "Volume 'neper_data' already exists, skipping creation"
    else
        log "Creating persistent volume..."
        flyctl volumes create neper_data \
            --region "$REGION" \
            --size "$VOLUME_SIZE" \
            --app "$APP_NAME" \
            --yes
    fi

    # Step 5: Set secrets
    log "Setting secrets..."

    # Generate secrets if not provided
    TOKEN_SECRET="${NEPER_TOKEN_SECRET:-$(openssl rand -hex 32)}"

    log "Generating NATS nkey..."
    NATS_NKEY="$(generate_nats_nkey)"

    flyctl secrets set --app "$APP_NAME" \
        NEPER_SERVE_TOKEN_SECRET="$TOKEN_SECRET" \
        NEPER_SERVE_NATS_CLIENT_NKEY="$NATS_NKEY" \
        NEPER_SERVE_AUTOCREATE_ADMIN="true" \
        NEPER_SERVE_ADMIN_USERNAME="$ADMIN_USERNAME" \
        NEPER_SERVE_ADMIN_EMAIL="$ADMIN_EMAIL"

    # Step 6: Show status
    log "Setup complete!"
    echo ""
    echo "=============================================="
    echo "Next steps:"
    echo "=============================================="
    echo ""
    echo "1. Deploy the app:"
    echo "   flyctl deploy --app $APP_NAME"
    echo ""
    echo "2. Or push a git tag to trigger CI deployment:"
    echo "   git tag v1.0.0 && git push origin v1.0.0"
    echo ""
    echo "3. View logs:"
    echo "   flyctl logs --app $APP_NAME"
    echo ""
    echo "4. Open the app:"
    echo "   flyctl open --app $APP_NAME"
    echo ""
    echo "=============================================="
    echo "Resources created:"
    echo "=============================================="
    flyctl apps list | grep "$APP_NAME" || true
    flyctl postgres list | grep "$DB_NAME" || true
    flyctl volumes list --app "$APP_NAME" 2>/dev/null || true
}

main "$@"
