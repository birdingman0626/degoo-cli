# degoo-cli Backend Architecture

## Module
`degoo-cli` — Go 1.26, module path `degoo-cli`

## Package Layout
- `internal/auth` — Login, token cache, credential resolution
- `internal/api` — GraphQL client, folder navigation
- `internal/httpclient` — Low-level JSON POST helper
- `internal/logger` — Dual-write logger (stdout + file)
- `internal/sync` — (Task 6+) recursive walk, timestamp compare, retry

## Degoo API Auth Flow

### Step 1: Login
```
POST https://rest-api.degoo.com/login
Body: {"Username": "...", "Password": "...", "GenerateToken": true}
Response: {"RefreshToken": "<opaque>", "UserInfo": {...}}
```
- `RefreshToken` is an opaque token (not a JWT)
- Stored in `TokenCache.RefreshToken`

### Step 2: Get JWT
```
POST https://rest-api.degoo.com/access-token/v2
Body: {"RefreshToken": "<opaque>"}
Response: {"AccessToken": "<JWT>"}
```
- JWT expires in ~1 hour
- Stored in `TokenCache.AccessToken`
- Token cache location: `%AppData%\degoo-cli\keys.json` (Windows)

### Step 3: GraphQL Calls
```
POST https://production-appsync.degoo.com/graphql
Headers: {"x-api-key": "da2-vs6twz5vnjdavpqndtbzg3prra", "Content-Type": "application/json"}
Body: {"query": "...", "variables": {"Token": "<JWT>", ...}}
```
- The JWT is passed as the `Token` argument inside GraphQL variables, NOT as an HTTP header
- `x-api-key` header is always required

## getFileChildren5 Query

```graphql
query getFileChildren5($Token: String!, $ParentID: String, $Limit: Int!, $Order: Int!, $NextToken: String) {
  getFileChildren5(Token: $Token, ParentID: $ParentID, Limit: $Limit, Order: $Order, NextToken: $NextToken) {
    Items {
      ID MetadataID Name FilePath Size Category LastModificationTime URL
    }
    NextToken
  }
}
```

Variables:
- `Token`: JWT access token (required)
- `ParentID`: folder ID string; use `"0"` for virtual root
- `Limit`: items per page (Int!, use 1000)
- `Order`: sort order (Int!, use 1)
- `NextToken`: pagination cursor (omit on first page)

## ContentView Type Fields
| Field | Type | Notes |
|-------|------|-------|
| ID | String | Primary file/folder ID |
| MetadataID | String | Alternate ID (fallback if ID empty) |
| Name | String | Display name |
| FilePath | String | Full path string |
| Size | String | File size as decimal string (e.g. "1048576") |
| Category | Int | 1=device folder, 4=user folder, 5=image, 6=document/text |
| LastModificationTime | String | Epoch milliseconds as decimal string |
| URL | String | Download URL for files |

## Category Values
- `1` — Device root folder (shown at root of tree) → `IsDirectory = true`
- `4` — Regular user-created folder → `IsDirectory = true`
- `5` — Image file
- `6` — Document/text file
- Other — Other file types

## Upload API Flow

Correct order (verified against real API): **setUploadFile3 → getBucketWriteAuth4 → GCS POST**

### Step 1: setUploadFile3 (register metadata first)
```graphql
mutation setUploadFile3($Token: String!, $FileInfos: [FileInfoUpload3]!) {
  setUploadFile3(Token: $Token, FileInfos: $FileInfos)
}
```
Input type `FileInfoUpload3` fields:
- `ParentID: String!` — parent folder ID
- `Name: String!` — filename
- `Size: String!` — file size as decimal string (**not Int**)
- `Checksum: String!` — Degoo SHA1 checksum (see Checksum Algorithm below)
- `CreationTime: String!` — Unix epoch **milliseconds** as string

Do NOT include a `Data` field — it causes "Request failed!" from the server.

Returns `Boolean`. Returns `true` on success.

### Step 2: getBucketWriteAuth4 (get GCS credentials)
```graphql
query getBucketWriteAuth4($Token: String!, $ParentID: String!, $StorageUploadInfos: [StorageUploadInfo2]) {
  getBucketWriteAuth4(Token: $Token, ParentID: $ParentID, StorageUploadInfos: $StorageUploadInfos) {
    AuthData {
      PolicyBase64
      Signature
      BaseURL
      KeyPrefix
      AccessKey { Key Value }
      ACL
      AdditionalBody { Key Value }
    }
    Error
  }
}
```
- Pass `StorageUploadInfos: []` (empty array, not omitted) — omitting it causes errors
- Returns GCS credentials for the given parent folder
- `KeyPrefix` is device-specific; object key = `KeyPrefix + filename`

### Step 3: Upload to Google Cloud Storage
- Multipart POST to `AuthData.BaseURL`
- Required form fields (in order): `key`, `acl`, `Content-Type`, `policy`, `GoogleAccessId`, `signature`, plus `AdditionalBody` fields
- **`Content-Type` form field is required** — missing it causes GCS to reject with `InvalidPolicyDocument`
- File content goes as `file` field with `Content-Type: application/octet-stream`
- Success: HTTP 204 No Content

## Download API

### getOverlay4 (get file metadata + signed URL)
```graphql
query GetOverlay4($Token: String!, $ID: IDType!) {
  getOverlay4(Token: $Token, ID: $ID) {
    ID URL
    # also: MetadataID UserID DeviceID Name FilePath Size Category
    # LastModificationTime ThumbnailURL Data IsInRecycleBin CreationTime LastUploadTime
    # NOT supported: OptimizedURL Distance Country Province Place Location GeoLocation IsShared ShareTime
  }
}
```
Variables:
- `Token`: JWT access token
- `ID`: IDType object — `{"FileID": <int64>}` — the integer file ID

Key facts:
- `getFileChildren5` NEVER populates the `URL` field (always empty)
- `getOverlay4` returns a signed `https://c.degoo.media/...` URL when the file is indexed
- Newly uploaded files may have an empty URL for minutes/hours after upload
- `GetFileURL(id string)` in client.go wraps this query

### File Download
- `DownloadFile(url string)` does a plain HTTP GET on the signed URL
- Files are stored in GCS bucket `degoo-production-large-file-us-east1.degoo.me` with ACL `project-private`; anonymous GET returns 403
- There is no `getBucketReadAuth` or equivalent — the signed URL IS the auth mechanism

## CreateFolder API

Uses `setUploadFile3` with `Size: "0"` and `Checksum: ""`. After the mutation
returns true, `GetChildren` is called to look up the new folder's ID.

## Checksum Algorithm
```
seed = [13, 7, 2, 2, 15, 40, 75, 117, 13, 10, 19, 16, 29, 23, 3, 36]
digest = sha1(seed + data)           # 20-byte SHA1
framed = [10, 20] + digest + [16, 0] # protobuf-like framing
degooChecksum(data) = base64(framed)
```

## Token Cache Schema (keys.json)
```json
{
  "accessToken": "<JWT — expires ~1h>",
  "refreshToken": "<opaque token — long-lived>"
}
```
