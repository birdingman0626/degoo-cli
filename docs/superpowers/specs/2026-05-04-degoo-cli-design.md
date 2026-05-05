# degoo-cli Design Spec
**Date:** 2026-05-04  
**Status:** Approved

## Overview

A cross-platform native CLI tool written in Go for uploading and downloading files to/from Degoo cloud storage. Designed to run headlessly on remote servers with recursive directory support, smart timestamp-based sync, retry logic, and dual stdout/file logging.

---

## Project Layout

```
degoo-cli/                        # renamed from Degoo_Tools
├── main.go
├── cmd/
│   ├── root.go                   # cobra root, global flags, .env loading
│   ├── upload.go                 # upload command
│   └── download.go               # download command
├── internal/
│   ├── auth/
│   │   └── auth.go               # login, token cache
│   ├── api/
│   │   └── client.go             # Degoo GraphQL + REST API calls
│   ├── sync/
│   │   └── sync.go               # recursive walk, timestamp compare, retry
│   └── logger/
│       └── logger.go             # dual-write: stdout + log file
├── .env                          # not committed
├── .gitignore
├── go.mod
└── .github/
    └── workflows/
        └── build.yml             # cross-platform build + release
```

**Dependencies:**
- `github.com/spf13/cobra` — CLI framework
- `github.com/joho/godotenv` — `.env` loading
- Standard `net/http` — all HTTP/GraphQL calls (no GraphQL framework)

---

## Commands

```
degoo upload <local-path> <remote-path> [flags]
degoo download <remote-path> <local-path> [flags]

Flags:
  --env string    path to .env file (default: .env beside the binary)
  --log string    path to log file  (default: degoo-cli.log beside the binary)
```

### Examples
```
degoo upload ./photos "/My Files/Photos"
degoo download "/My Files/Photos" ./photos
degoo upload ./backup /Backups --env /etc/degoo/.env
```

---

## Credential Resolution

Order of precedence:
1. `.env` file beside the binary (`USER` and `PASSWORD` keys)
2. Override path via `--env` flag
3. Interactive prompt — if credentials not found, ask once at startup

After successful login, the access/refresh token is cached at `~/.config/degoo-cli/keys.json`. Subsequent runs skip login and auto-refresh the token if expired.

---

## Degoo API

**Endpoints:**
- Login: `POST https://rest-api.degoo.com/login`
- Token refresh: `POST https://rest-api.degoo.com/access-token/v2`
- All file operations: `POST https://production-appsync.degoo.com/graphql`

**Auth headers on every GraphQL request:**
- `x-api-key: da2-vs6twz5vnjdavpqndtbzg3prra`
- `Token: <access_token>`

**Key operations used:**
- `getFileChildren5` — list directory contents (paginated)
- `setUploadFile3` + `getBucketWriteAuth4` — create file metadata + get upload URL
- File download — via URL returned in file metadata

---

## Sync Logic

### Upload (per file)
1. Resolve remote path; create intermediate Degoo folders if missing
2. Fetch remote file metadata — get `LastModificationTime`
3. Compare with local `mtime`:
   - Remote same age or newer → **skip**
   - Local is newer → **upload**
4. Upload: call `setUploadFile3` → `getBucketWriteAuth4` → PUT to cloud storage URL
5. Checksum: SHA1 hash seeded with Degoo's hardcoded bytes, base64-encoded
6. On failure: retry up to 3 times with exponential backoff (1s → 2s → 4s); on max retries log as failed and continue

### Download (per file)
1. Walk remote directory via `getFileChildren5` (recursive, handles pagination)
2. For each file, compare remote `LastModificationTime` vs local `mtime`:
   - Local same age or newer → **skip**
   - Remote is newer → **download**
3. GET file from URL in metadata
4. Same 3-retry logic on failure

---

## Logging

- Every log line printed to **stdout** and **appended to log file** simultaneously
- Log file location: beside the binary as `degoo-cli.log` (override with `--log`)
- Format: `[2006-01-02 15:04:05] [INFO/WARN/ERROR] message`
- Examples:
  ```
  [2026-05-04 10:00:01] [INFO] Starting upload: ./photos → /My Files/Photos
  [2026-05-04 10:00:02] [INFO] [1/42] Uploading vacation.jpg (3.2 MB)
  [2026-05-04 10:00:05] [INFO] [2/42] Skipping old-photo.jpg (remote is newer)
  [2026-05-04 10:01:10] [WARN] [5/42] Failed to upload broken.jpg (attempt 1/3), retrying...
  [2026-05-04 10:01:44] [ERROR] [5/42] broken.jpg failed after 3 attempts
  ```

### Summary Report (end of run)
```
=== Transfer Summary ===
Uploaded:  42 files (1.2 GB)
Skipped:   15 files (already up to date)
Failed:     2 files
  - /photos/broken.jpg       (max retries exceeded)
  - /docs/report.pdf         (max retries exceeded)
=======================
```

---

## GitHub Actions Build Pipeline

**Triggers:**
- Push to `main` — build and verify all 6 binaries
- Push tag `v*.*.*` — build + create GitHub Release with all binaries attached

**Build matrix** (all built on a single Ubuntu runner via Go cross-compilation):

| OS      | Arch  | Output filename                  |
|---------|-------|----------------------------------|
| Linux   | amd64 | `degoo-cli-linux-amd64`          |
| Linux   | arm64 | `degoo-cli-linux-arm64`          |
| macOS   | amd64 | `degoo-cli-darwin-amd64`         |
| macOS   | arm64 | `degoo-cli-darwin-arm64`         |
| Windows | amd64 | `degoo-cli-windows-amd64.exe`    |
| Windows | arm64 | `degoo-cli-windows-arm64.exe`    |

**`.gitignore` includes:** `/go/`, `.env`, `*.log`, compiled binaries

---

## Out of Scope

- File listing, renaming, moving, deleting (managed via Degoo web UI)
- Interactive TUI or progress bars
- Parallel concurrent uploads/downloads (sequential per file)
