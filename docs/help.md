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
**POST /api/sync-all** - Sync all accounts

## Reading Email

Browse emails by folder. The app syncs messages from IMAP and stores them locally for fast access.

**GET /api/folders** - List all folders
**GET /api/emails** - List emails (filter by folder, account)
**GET /api/emails/{id}** - Read a full email with body
**GET /api/thread** - View a conversation thread

## Email Actions

**POST /api/emails/{id}/star** - Toggle star on an email
**POST /api/emails/{id}/unread** - Mark an email as unread
**POST /api/emails/{id}/unsubscribe** - Unsubscribe from a mailing list
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

### Version

```bash
./email-server --version
```

### Build from source

```bash
# Development (native)
cd apps/email && go build -o bin/email-server ./cmd/email-server

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "-s -w" -trimpath -o bin/email-server-linux-amd64 ./cmd/email-server
```

### Docker

Build a Docker image directly from the binary:

```bash
# Default base image (debian:12-slim)
./email-server docker-build

# Custom base image
./email-server docker-build --base ubuntu:24.04

# Custom Dockerfile
./email-server docker-build --dockerfile ./my.Dockerfile

# Tag and push to registry
./email-server docker-build --tag ghcr.io/localitas/email:latest --push
```

The `docker-build` command requires a Linux amd64 binary in the same directory. Run `make deploy-build` from the project root first.

### Download

Pre-built binaries are available on the [GitHub releases page](https://github.com/localitas/localitas/releases).

Each release includes three builds per app:
- `email-server-darwin-arm64` (macOS Apple Silicon)
- `email-server-linux-amd64` (Linux x86_64)
- `email-server-linux-arm64` (Linux ARM64)

Download with the GitHub CLI:

    gh release download --repo localitas/localitas --pattern 'email-server-*'

### Release

All app binaries are published to GitHub releases as part of `make deploy-upload-image`.
