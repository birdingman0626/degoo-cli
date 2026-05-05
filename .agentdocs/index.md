# degoo-cli Agent Documentation Index

## Backend Documentation
`backend/architecture.md` - Backend architecture, API auth flow, GraphQL schema facts; must read before modifying any backend code

## Global Important Memories

- The `RefreshToken` from Degoo login (`/login`) is an opaque token, NOT a JWT. It must be exchanged via `POST /access-token/v2` with body `{"RefreshToken": "<opaque>"}` to obtain a short-lived JWT `AccessToken` that can be passed to GraphQL queries.
- All Degoo GraphQL queries pass the JWT as the `Token` argument (not as an HTTP header). The `x-api-key` header is also required.
- `getFileChildren5` argument names: `Token` (String! — JWT), `ParentID` (String, nullable), `Limit` (Int!), `Order` (Int!), `NextToken` (String, nullable for pagination). Returns `ContentViewConnection { Items [ContentView] NextToken String }`.
- `ContentView` fields used: `ID`, `MetadataID`, `Name`, `FilePath`, `Size` (String), `Category` (Int), `LastModificationTime` (String, epoch ms), `URL`.
- Degoo Category values: `1` = device root folder (navigable), `4` = regular folder (navigable), `6` = document/text file. Both 1 and 4 are treated as `IsDirectory = true`.
- Upload uses 3-step flow: `getBucketWriteAuth4` (no StorageUploadInfos!) → GCS multipart POST (must include `Content-Type` form field) → `setUploadFile3`. Input type is `FileInfoUpload3`; `Size` and `CreationTime` are Strings, not Ints.
- `setUploadFile3` returns "Request failed!" for Web-device JWT tokens — a known Degoo backend restriction. GCS upload step works fine.
- `degooChecksum(data) = base64(0x02 || sha1(0x02 || data))`
