package api

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

)

const (
	graphqlURL = "https://production-appsync.degoo.com/graphql"
	// apiKey is the public AppSync API key shared by all Degoo clients — not a secret.
	apiKey = "da2-vs6twz5vnjdavpqndtbzg3prra"
)

// FileInfo represents a file or folder in Degoo.
type FileInfo struct {
	ID           string
	Name         string
	FilePath     string
	Size         int64
	IsDirectory  bool
	ModifiedTime time.Time
	DownloadURL  string
}

// Client is a Degoo GraphQL API client.
type Client struct {
	token string
}

// NewClient creates a new API client using the given access token.
func NewClient(token string) *Client {
	return &Client{token: token}
}

// graphql executes a GraphQL operation and unmarshals the data field into out.
func (c *Client) graphql(operationName, query string, variables map[string]interface{}, out interface{}) error {
	body := map[string]interface{}{
		"operationName": operationName,
		"variables":     variables,
		"query":         query,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, graphqlURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, raw)
	}

	var rawResult struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &rawResult); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if len(rawResult.Errors) > 0 {
		return fmt.Errorf("graphql error: %s", rawResult.Errors[0].Message)
	}
	return json.Unmarshal(rawResult.Data, out)
}

// rawNode maps the ContentView type returned by getFileChildren5.
// All time and size fields come back as strings from the API.
type rawNode struct {
	ID                   string `json:"ID"`
	MetadataID           string `json:"MetadataID"`
	Name                 string `json:"Name"`
	FilePath             string `json:"FilePath"`
	Size                 string `json:"Size"`
	Category             int    `json:"Category"`
	LastModificationTime string `json:"LastModificationTime"`
	URL                  string `json:"URL"`
}

// childrenQuery passes the auth token as the required Token argument.
// Pagination uses NextToken (optional String).
const childrenQuery = `
query getFileChildren5($Token: String!, $ParentID: String, $Limit: Int!, $Order: Int!, $NextToken: String) {
  getFileChildren5(Token: $Token, ParentID: $ParentID, Limit: $Limit, Order: $Order, NextToken: $NextToken) {
    Items {
      ID MetadataID Name FilePath Size Category LastModificationTime URL
    }
    NextToken
  }
}`

// GetChildren returns the children of the folder with the given ID.
// Use "0" for the root.
func (c *Client) GetChildren(folderID string) ([]FileInfo, error) {
	var all []FileInfo
	var nextToken *string

	for {
		vars := map[string]interface{}{
			"Token":    c.token,
			"ParentID": folderID,
			"Limit":    1000,
			"Order":    1,
		}
		if nextToken != nil {
			vars["NextToken"] = *nextToken
		}

		var result struct {
			GetFileChildren5 struct {
				Items     []rawNode `json:"Items"`
				NextToken *string   `json:"NextToken"`
			} `json:"getFileChildren5"`
		}
		if err := c.graphql("getFileChildren5", childrenQuery, vars, &result); err != nil {
			return nil, err
		}

		for _, n := range result.GetFileChildren5.Items {
			all = append(all, toFileInfo(n))
		}

		nextToken = result.GetFileChildren5.NextToken
		if nextToken == nil || *nextToken == "" {
			break
		}
	}
	return all, nil
}

// toFileInfo converts a rawNode into a FileInfo.
// Category 4 = folder in Degoo's schema.
func toFileInfo(n rawNode) FileInfo {
	id := n.ID
	if id == "" {
		id = n.MetadataID
	}

	// Size comes back as a decimal string (e.g. "1048576")
	var size int64
	fmt.Sscanf(n.Size, "%d", &size)

	// LastModificationTime is epoch milliseconds as a decimal string
	var mtime time.Time
	var ms int64
	scanned, _ := fmt.Sscanf(n.LastModificationTime, "%d", &ms)
	if scanned == 1 && ms > 0 {
		mtime = time.UnixMilli(ms)
	}

	// Category 1 = device root folder, Category 2 = folder, Category 4 = device subfolder.
	isDir := n.Category == 1 || n.Category == 2 || n.Category == 4

	return FileInfo{
		ID:           id,
		Name:         n.Name,
		FilePath:     n.FilePath,
		Size:         size,
		IsDirectory:  isDir,
		ModifiedTime: mtime,
		DownloadURL:  n.URL,
	}
}

// ResolveRemotePath walks the Degoo folder tree to find the ID of remotePath.
// "/" resolves to the top-level folder (first root child, typically "My Files").
func (c *Client) ResolveRemotePath(remotePath string) (string, error) {
	parts := splitPath(remotePath)

	children, err := c.GetChildren("0")
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}

	// "/" with no parts → return the first root child's ID
	if len(parts) == 0 {
		if len(children) == 0 {
			return "0", nil
		}
		return children[0].ID, nil
	}
	if len(children) == 0 {
		return "", fmt.Errorf("no root folders found")
	}

	// Walk each path segment, anchored at the first root child ("My Files")
	currentID := children[0].ID
	for _, part := range parts {
		if part == "" {
			continue
		}
		items, err := c.GetChildren(currentID)
		if err != nil {
			return "", fmt.Errorf("resolve %q: %w", part, err)
		}
		found := false
		for _, item := range items {
			if strings.EqualFold(item.Name, part) && item.IsDirectory {
				currentID = item.ID
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("path segment %q not found under ID %s", part, currentID)
		}
	}
	return currentID, nil
}

// EnsureRemotePath walks the tree creating missing folders as needed.
func (c *Client) EnsureRemotePath(remotePath string) (string, error) {
	parts := splitPath(remotePath)

	children, err := c.GetChildren("0")
	if err != nil {
		return "", err
	}
	if len(parts) == 0 {
		if len(children) == 0 {
			return "0", nil
		}
		return children[0].ID, nil
	}
	if len(children) == 0 {
		return "", fmt.Errorf("no root folders found")
	}

	// Start from the first root child ("My Files")
	currentID := children[0].ID

	for _, part := range parts {
		if part == "" {
			continue
		}
		items, err := c.GetChildren(currentID)
		if err != nil {
			return "", fmt.Errorf("ensure %q: %w", part, err)
		}
		found := ""
		for _, item := range items {
			if strings.EqualFold(item.Name, part) && item.IsDirectory {
				found = item.ID
				break
			}
		}
		if found != "" {
			currentID = found
			continue
		}
		newID, err := c.CreateFolder(currentID, part)
		if err != nil {
			return "", fmt.Errorf("create folder %q: %w", part, err)
		}
		currentID = newID
	}
	return currentID, nil
}

// UploadRequest holds the parameters for uploading a single file.
// Open is called up to twice: once to compute the Degoo checksum, once to stream bytes to GCS.
type UploadRequest struct {
	ParentID string
	Name     string
	Size     int64
	MTime    time.Time
	Open     func() (io.ReadCloser, error)
}

// degooChecksum computes the Degoo-specific SHA1 checksum by streaming from open().
// Format: base64([10, 20, ...sha1(seed+data)..., 16, 0])
func degooChecksum(open func() (io.ReadCloser, error)) (string, error) {
	r, err := open()
	if err != nil {
		return "", err
	}
	defer r.Close()
	seed := []byte{13, 7, 2, 2, 15, 40, 75, 117, 13, 10, 19, 16, 29, 23, 3, 36}
	h := sha1.New()
	h.Write(seed)
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	digest := h.Sum(nil) // 20 bytes
	cs := make([]byte, 0, 2+len(digest)+2)
	cs = append(cs, 10, byte(len(digest)))
	cs = append(cs, digest...)
	cs = append(cs, 16, 0)
	return base64.StdEncoding.EncodeToString(cs), nil
}

// uploadFileMutation registers file metadata with Degoo.
// FileInfoUpload3.Size is a String, CreationTime is epoch-ms as String.
// The mutation returns Boolean (true on success).
const uploadFileMutation = `
mutation setUploadFile3($Token: String!, $FileInfos: [FileInfoUpload3]!) {
  setUploadFile3(Token: $Token, FileInfos: $FileInfos)
}`

// bucketAuthQuery fetches cloud storage upload credentials for the parent folder.
// StorageUploadInfos must be passed as an empty array (not omitted) per the Python reference.
const bucketAuthQuery = `
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
}`

// keyValue is a generic key-value pair returned by the Degoo API.
type keyValue struct {
	Key   string `json:"Key"`
	Value string `json:"Value"`
}

// bucketWriteAuth holds the Google Cloud Storage pre-signed POST credentials.
type bucketWriteAuth struct {
	PolicyBase64   string     `json:"PolicyBase64"`
	Signature      string     `json:"Signature"`
	BaseURL        string     `json:"BaseURL"`
	KeyPrefix      string     `json:"KeyPrefix"`
	AccessKey      keyValue   `json:"AccessKey"`
	ACL            string     `json:"ACL"`
	AdditionalBody []keyValue `json:"AdditionalBody"`
}

// UploadFile uploads a file to Degoo under the given parent folder.
// Flow: (1) compute Degoo checksum by streaming the file, (2) register metadata via
// setUploadFile3, (3) get GCS credentials via getBucketWriteAuth4, (4) stream file to GCS.
func (c *Client) UploadFile(req UploadRequest) error {
	checksum, err := degooChecksum(req.Open)
	if err != nil {
		return fmt.Errorf("checksum: %w", err)
	}
	sizeStr := strconv.FormatInt(req.Size, 10)
	creationTime := strconv.FormatInt(req.MTime.UnixMilli(), 10)

	// Step 1: register the file metadata with Degoo.
	var uploadResult struct {
		SetUploadFile3 bool `json:"setUploadFile3"`
	}
	err = c.graphql("setUploadFile3", uploadFileMutation, map[string]interface{}{
		"Token": c.token,
		"FileInfos": []map[string]interface{}{
			{
				"ParentID":     req.ParentID,
				"Name":         req.Name,
				"Size":         sizeStr,
				"Checksum":     checksum,
				"CreationTime": creationTime,
			},
		},
	}, &uploadResult)
	if err != nil {
		return fmt.Errorf("setUploadFile3: %w", err)
	}
	if !uploadResult.SetUploadFile3 {
		return fmt.Errorf("setUploadFile3: server returned false")
	}

	// Step 2: get cloud storage credentials.
	var authResult struct {
		GetBucketWriteAuth4 []struct {
			AuthData bucketWriteAuth `json:"AuthData"`
			Error    string          `json:"Error"`
		} `json:"getBucketWriteAuth4"`
	}
	if err := c.graphql("getBucketWriteAuth4", bucketAuthQuery, map[string]interface{}{
		"Token":              c.token,
		"ParentID":           req.ParentID,
		"StorageUploadInfos": []interface{}{},
	}, &authResult); err != nil {
		return fmt.Errorf("getBucketWriteAuth4: %w", err)
	}
	if len(authResult.GetBucketWriteAuth4) == 0 {
		return fmt.Errorf("getBucketWriteAuth4: empty response")
	}
	slot := authResult.GetBucketWriteAuth4[0]
	if slot.Error != "" {
		return fmt.Errorf("getBucketWriteAuth4 server error: %s", slot.Error)
	}
	a := slot.AuthData
	objectKey := a.KeyPrefix + req.Name

	// Step 3: stream file bytes to Google Cloud Storage.
	r, err := req.Open()
	if err != nil {
		return fmt.Errorf("open for upload: %w", err)
	}
	defer r.Close()
	if err := c.putToStorage(a.BaseURL, objectKey, req.Name, req.Size, r, a); err != nil {
		return fmt.Errorf("storage upload: %w", err)
	}
	return nil
}

// putToStorage streams file bytes to Google Cloud Storage via multipart POST.
// The GCS policy requires Content-Type to be present as a form field.
// The file reader is streamed directly — no full-file buffer is held in memory.
func (c *Client) putToStorage(baseURL, objectKey, filename string, size int64, r io.Reader, a bucketWriteAuth) error {
	boundary := fmt.Sprintf("degoo-cli-boundary-%x", time.Now().UnixNano())

	// GCS pre-signed POST fields (order matters for policy validation).
	var prefix bytes.Buffer
	formFields := []keyValue{
		{Key: "key", Value: objectKey},
		{Key: "acl", Value: a.ACL},
		{Key: "Content-Type", Value: "application/octet-stream"},
		{Key: "policy", Value: a.PolicyBase64},
		{Key: a.AccessKey.Key, Value: a.AccessKey.Value},
		{Key: "signature", Value: a.Signature},
	}
	formFields = append(formFields, a.AdditionalBody...)

	for _, f := range formFields {
		if f.Key == "" {
			continue
		}
		fmt.Fprintf(&prefix, "--%s\r\nContent-Disposition: form-data; name=%q\r\n\r\n%s\r\n", boundary, f.Key, f.Value)
	}
	fmt.Fprintf(&prefix, "--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=%q\r\nContent-Type: application/octet-stream\r\n\r\n", boundary, filename)

	suffix := []byte(fmt.Sprintf("\r\n--%s--\r\n", boundary))
	totalLen := int64(prefix.Len()) + size + int64(len(suffix))

	// Stream the multipart body: headers prefix + file bytes + closing boundary.
	body := io.MultiReader(&prefix, r, bytes.NewReader(suffix))
	httpReq, err := http.NewRequest(http.MethodPost, baseURL, body)
	if err != nil {
		return err
	}
	httpReq.ContentLength = totalLen
	httpReq.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

// CreateFolder creates a new folder under parentID using setUploadFile3.
// Degoo treats a zero-size file with empty checksum as a folder.
// Returns the folder name (Degoo does not return a new ID from this mutation).
func (c *Client) CreateFolder(parentID, name string) (string, error) {
	var result struct {
		SetUploadFile3 bool `json:"setUploadFile3"`
	}
	err := c.graphql("setUploadFile3", uploadFileMutation, map[string]interface{}{
		"Token": c.token,
		"FileInfos": []map[string]interface{}{
			{
				"ParentID":     parentID,
				"Name":         name,
				"Size":         "0",
				"Checksum":     "CgAQAg",
				"CreationTime": strconv.FormatInt(time.Now().UnixMilli(), 10),
			},
		},
	}, &result)
	if err != nil {
		return "", fmt.Errorf("createFolder %q: %w", name, err)
	}
	if !result.SetUploadFile3 {
		return "", fmt.Errorf("createFolder %q: server returned false", name)
	}
	// The mutation returns a bool, not a new ID.
	// Degoo may lag slightly before the new folder appears in listings; retry briefly.
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		items, err := c.GetChildren(parentID)
		if err != nil {
			return "", fmt.Errorf("createFolder: lookup new folder %q: %w", name, err)
		}
		for _, item := range items {
			if strings.EqualFold(item.Name, name) && item.IsDirectory {
				return item.ID, nil
			}
		}
	}
	return "", fmt.Errorf("createFolder: folder %q not found after creation", name)
}

// getOverlay4Query fetches per-file metadata including the signed download URL.
// ID must be an integer file ID passed as IDType: {"FileID": <int>}.
const getOverlay4Query = `
query GetOverlay4($Token: String!, $ID: IDType!) {
  getOverlay4(Token: $Token, ID: $ID) {
    ID URL
  }
}`

// GetFileURL fetches the signed download URL for the file with the given ID
// by calling getOverlay4. Returns an error if the URL is empty (file not yet indexed).
func (c *Client) GetFileURL(id string) (string, error) {
	fileID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return "", fmt.Errorf("invalid file ID %q: %w", id, err)
	}
	var result struct {
		GetOverlay4 struct {
			ID  string `json:"ID"`
			URL string `json:"URL"`
		} `json:"getOverlay4"`
	}
	if err := c.graphql("GetOverlay4", getOverlay4Query, map[string]interface{}{
		"Token": c.token,
		"ID":    map[string]interface{}{"FileID": fileID},
	}, &result); err != nil {
		return "", fmt.Errorf("getOverlay4: %w", err)
	}
	if result.GetOverlay4.URL == "" {
		return "", fmt.Errorf("getOverlay4: empty URL for file ID %s (file may not be indexed yet)", id)
	}
	return result.GetOverlay4.URL, nil
}

// DownloadFile streams the file at url into dst.
func (c *Client) DownloadFile(url string, dst io.Writer) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download HTTP %d: %s", resp.StatusCode, body)
	}
	_, err = io.Copy(dst, resp.Body)
	return err
}

func splitPath(p string) []string {
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}
