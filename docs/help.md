---
title: Email
description: Email client with IMAP/SMTP support
---

# Email

Read, compose, and manage email through IMAP and SMTP with multi-account support.

## Accounts

Add and manage email accounts. Each account stores IMAP and SMTP connection details. Google OAuth is supported for Gmail accounts.

**GET /api/accounts** - List all accounts
**POST /api/accounts** - Add an email account
**PUT /api/accounts/{id}** - Update account settings
**DELETE /api/accounts/{id}** - Remove an account
**POST /api/accounts/{id}/sync** - Sync a single account
**POST /api/sync-all** - Sync every account (the scheduled job; only core may call it)

## Reading Email

Browse emails by folder. The app syncs messages from IMAP and stores them locally for fast access.

**GET /api/folders** - List all folders
**GET /api/emails** - List emails (filter by folder, account)
**GET /api/emails/{id}** - Read a full email with body
**GET /api/thread** - View a conversation thread

## Email Actions

**POST /api/emails/{id}/star** - Toggle star on an email
**POST /api/emails/{id}/unread** - Mark an email as unread
**POST /api/emails/{id}/unsubscribe** - Unsubscribe from a mailing list, using the List-Unsubscribe target the email carries
**DELETE /api/emails/{id}** - Move an email to trash
**POST /api/emails/{id}/restore** - Restore an email from trash
**GET /api/trash** - List trashed emails

## Composing Email

Send new emails or reply to existing ones via SMTP.

**POST /api/compose** - Send an email

## Drafts

Save and manage email drafts before sending.

**POST /api/drafts** - Save a draft
**GET /api/drafts** - List all drafts
**GET /api/drafts/{id}** - Load a draft
**DELETE /api/drafts/{id}** - Delete a draft

## Attachments

**GET /api/emails/{id}/attachments** - List attachments on an email
**GET /api/attachments/{aid}/download** - Download an attachment

## Filters

Create rules to automatically organize incoming email.

**GET /api/filters** - List filters
**POST /api/filters** - Create a filter
**PUT /api/filters/{id}** - Update a filter
**DELETE /api/filters/{id}** - Delete a filter
**POST /api/filters/test** - Test a filter against sample emails

## Search

**GET /api/search** - Full-text search across all emails

## Build & Deploy

Run these in the app's repository, `~/localitas-app-email`.

### Version

```bash
bin/email-server --version
```

### Build from source

```bash
# Native binary (bin/email-server)
make build

# Linux amd64 binary the Docker image is built from (email-server-linux-amd64)
make build-linux
```

Both run `make lint` (gofmt and go vet) first; `make test` runs the lint step and then the tests.

### Run natively

```bash
make start   # builds bin/email-server, writes the launchd service com.localitas.app.email for bin/email-server and loads it on port 9208
make stop
make logs    # follows ~/.localitas/logs/email/stdout.log
```

The app announces itself to core when it starts, and core serves it at `/apps/ext/email/`.

### Docker

```bash
make build-docker   # builds the Linux binary and the email:latest image
make start-docker   # runs container "email" on port 9208, labelled localitas.app=true
make stop-docker
make logs-docker
```

Core registers containers labelled `localitas.app=true` once Docker reports them healthy.

### Publish

```bash
# Run the tests, build the image, push ghcr.io/localitas/localitas-app-email:latest and :<VERSION>
make docker-push

# Build dist/email-server-darwin-arm64 and dist/email-server-darwin-amd64
# and create GitHub release vX.Y.Z on this repository with them
make release VERSION=vX.Y.Z
```

`VERSION` defaults to `git describe --tags --always --dirty`.

### Download

Pre-built macOS binaries are on the [releases page](https://github.com/localitas/localitas-app-email/releases):
- `email-server-darwin-arm64` (Apple Silicon)
- `email-server-darwin-amd64` (Intel)

Every push to `main` publishes a release: `.github/workflows/release.yml` runs the tests and releases both binaries as `v0.1.<commit count>.<UTC timestamp>`. `make release` publishes one by hand.

Download with the GitHub CLI:

    gh release download --repo localitas/localitas-app-email --pattern 'email-server-*'
