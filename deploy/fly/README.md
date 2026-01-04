# Fly.io Deployment

Infrastructure-as-code setup for deploying Neper to Fly.io.

## Prerequisites

1. Install flyctl:
   ```bash
   curl -L https://fly.io/install.sh | sh
   ```

2. Authenticate:
   ```bash
   flyctl auth login
   ```

3. Go installed (for NATS nkey generation), or pre-generate the key:
   ```bash
   # Option A: Have Go installed (setup.sh will use `go run`)
   # Option B: Pre-generate and export:
   go run ./cmd/neper generate-nkey --type user
   export NEPER_NATS_NKEY="SUABC..."  # the seed line from output
   ```

## Quick Start

### 1. Create Infrastructure (one-time)

```bash
# Optional: customize settings
export NEPER_FLY_APP_NAME=neper
export NEPER_FLY_REGION=cdg  # Paris
export NEPER_ADMIN_EMAIL=admin@yourdomain.com

# Run setup
./deploy/fly/setup.sh
```

### 2. Deploy

```bash
# Manual deploy
flyctl deploy

# Or via Git tag (triggers GitHub Actions)
git tag v1.0.0
git push origin v1.0.0
```

## Configuration

### Environment Variables for setup.sh

| Variable                | Default             | Description                 |
|-------------------------|---------------------|-----------------------------|
| `NEPER_FLY_APP_NAME`    | `neper`             | Fly.io app name             |
| `NEPER_FLY_DB_NAME`     | `neper-db`          | Postgres database name      |
| `NEPER_FLY_REGION`      | `cdg`               | Deployment region           |
| `NEPER_FLY_VOLUME_SIZE` | `1`                 | Volume size in GB           |
| `NEPER_ADMIN_USERNAME`  | `admin`             | Auto-created admin username |
| `NEPER_ADMIN_EMAIL`     | `admin@neper.local` | Admin email                 |

### Secrets (set automatically by setup.sh)

| Secret                         | Description                     |
|--------------------------------|---------------------------------|
| `DATABASE_URL`                 | Set by `flyctl postgres attach` |
| `NEPER_SERVE_TOKEN_SECRET`     | JWT signing key                 |
| `NEPER_SERVE_NATS_CLIENT_NKEY` | NATS nkey seed (starts with SU) |
| `NEPER_SERVE_AUTOCREATE_ADMIN` | Enable admin auto-creation      |

## GitHub Actions CI/CD

The workflow in `.github/workflows/fly-deploy.yml` deploys on:
- Git tags matching `v*`
- Manual trigger from GitHub UI

### Setup GitHub Environment

The workflow uses a GitHub environment called `production` for deployment protection and secrets isolation.

1. **Create the environment:**
   - Go to your repository on GitHub
   - Navigate to **Settings** → **Environments**
   - Click **New environment**
   - Name: `production`
   - Click **Configure environment**

2. **Optional: Add protection rules**
   - Enable **Required reviewers** to require approval before deploys
   - Enable **Wait timer** to add a delay before deployment starts
   - Restrict **Deployment branches** to only allow tags or specific branches

3. **Generate Fly.io deploy token:**
   ```bash
   flyctl tokens create deploy -x 999999h
   ```

4. **Add secret to the environment:**
   - In the environment configuration page, scroll to **Environment secrets**
   - Click **Add secret**
   - Name: `FLY_API_TOKEN`
   - Value: paste the token from step 3
   - Click **Add secret**

> **Note:** Environment secrets are only available to jobs that reference that environment. This is more secure than repository-level secrets as it allows for additional protection rules.

## Useful Commands

```bash
# View logs
flyctl logs --app neper

# SSH into the machine
flyctl ssh console --app neper

# Open in browser
flyctl open --app neper

# Check status
flyctl status --app neper

# Scale up
flyctl scale count 2 --app neper

# Connect to Postgres
flyctl postgres connect --app neper-db
```

## Teardown

```bash
# WARNING: Destroys everything including data!
./deploy/fly/teardown.sh
```

## Cost Estimate

| Resource                   | Monthly Cost |
|----------------------------|--------------|
| App (shared-cpu-1x, 512MB) | ~$3          |
| Postgres (shared, 1GB)     | ~$7          |
| Volume (1GB)               | ~$0.15       |
| **Total**                  | **~$10**     |
