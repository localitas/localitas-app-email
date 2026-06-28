# Email

Email client with IMAP/SMTP support.

Part of the [Localitas](https://github.com/localitas) platform — a self-hosted, privacy-first personal computing system.

## Features

- IMAP email sync with folder management
- SMTP sending with draft support
- Google OAuth for Gmail accounts
- Email threading and attachment handling
- Server-side filtering rules with custom DSL
- Dangerous email detection and unsubscribe support

## Installation

### Development (via Localitas core)

```bash
# Clone the repo
git clone https://github.com/localitas/localitas-app-email.git ~/localitas-app-email

# Start with the Localitas dev cluster (builds and runs in Docker automatically)
cd ~/localitas && make dev-core
```

### Standalone

```bash
cd ~/localitas-app-email

# Build and run locally
make build
./bin/email-server serve --listen :8000

# Or via launchd (macOS)
make start

# Or via Docker
make start-docker
```

## Exposing to the Internet

Localitas apps are accessible remotely through Localitas's built-in tunnel service, powered by FRP. No port forwarding or dynamic DNS required.

1. Sign up at [localitas.com](https://localitas.com) and connect your local Localitas core
2. The tunnel automatically exposes your core (and all apps) at `https://{your-subdomain}.localitas.com`
3. This app is available at `https://{your-subdomain}.localitas.com/apps/ext/email/`

All traffic is encrypted end-to-end. Authentication is handled by the Localitas core — only authorized users can access your apps.

## App Store

Install via the Localitas App Store (recommended):

```bash
localitas-core app-store add --name email --compose ./docker-compose.yml --port 9208
localitas-core app-store start email
```

Or open the App Store UI (package icon, top-right nav) and paste the `docker-compose.yml`.

The image is published to `ghcr.io/localitas/localitas-app-email:latest`. To publish a new version:

```bash
make docker-push   # runs tests, builds, and pushes to ghcr.io
```

## License

MIT
