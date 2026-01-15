# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A reverse proxy gateway that serves static files from a Quartz-based static site with Casdoor OAuth authentication. Protected routes require login via Casdoor identity provider.

## Build and Run

```bash
# Development
go run main.go

# Production build (Windows)
go build -o quartz-gw.exe main.go
./quartz-gw.exe
```

## Configuration

Edit `config.json` with your Casdoor credentials:

| Field | Description |
|-------|-------------|
| `listen_addr` | Server listen address (`:8766`) |
| `base_url` | Public gateway URL |
| `quartz_dir` | Path to Quartz output directory |
| `casdoor_addr` | Casdoor server URL |
| `client_id`, `client_secret` | OAuth credentials |
| `app_name` | OAuth application name |
| `redirect_path` | OAuth callback URL |

## Architecture

**Single-file Go application** using only standard library.

**Request flow:**
1. `/` → `handleMain()` - Checks `quartz_session` cookie, serves static files
2. `/callback` → `handleCallback()` - Exchanges OAuth code for token, sets auth cookies
3. `/logout` → `handleLogout()` - Clears cookies, redirects to home

**Key functions:**
- `checkAuth()` - Verifies `quartz_session` cookie
- `fetchRealUsername()` - Exchanges OAuth code for JWT, extracts username from payload
- `serveQuartzFile()` - Serves static files with cache headers
- `isStaticResource()` - Distinguishes JS/CSS/images from HTML pages

**Cookie-based auth:**
- `quartz_session` (HttpOnly) - Authentication state
- `quartz_username` - Display name for UI

**Path handling:**
- `/` or `/folder/` → serves `index.html`
- `/page` → serves `page.html`
- HTML files: `Cache-Control: no-store` (prevents cached page after logout)
- Static resources: `Cache-Control: max-age=31536000`

**Static resource protection:** Non-HTML resources return HTTP 401 (not redirect) to avoid HTML parsing errors when browser fetches resources while unauthenticated.
