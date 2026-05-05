package api_test

import (
	"bytes"
	"io"
	"testing"
	"time"

	"degoo-cli/internal/api"
	"degoo-cli/internal/auth"
)

func getTestClient(t *testing.T) *api.Client {
	t.Helper()
	cache, err := auth.LoadTokenCache()
	if err != nil || cache.AccessToken == "" {
		t.Skip("no token cache — run TestLoginRealAPI first")
	}
	return api.NewClient(cache.AccessToken)
}

func TestGetRootChildren(t *testing.T) {
	client := getTestClient(t)
	items, err := client.GetChildren("0")
	if err != nil {
		t.Fatalf("GetChildren: %v", err)
	}
	if len(items) == 0 {
		t.Error("expected at least one item in root")
	}
	t.Logf("root has %d items, first: %+v", len(items), items[0])
}

func TestResolveRemotePath(t *testing.T) {
	client := getTestClient(t)
	id, err := client.ResolveRemotePath("/")
	if err != nil {
		t.Fatalf("ResolveRemotePath: %v", err)
	}
	if id == "" {
		t.Error("expected non-empty ID")
	}
	t.Logf("root ID: %s", id)
}

func TestUploadSmallFile(t *testing.T) {
	client := getTestClient(t)

	content := []byte("degoo-cli upload test " + time.Now().String())

	rootID, err := client.ResolveRemotePath("/")
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	t.Logf("uploading to root folder ID: %s", rootID)

	err = client.UploadFile(api.UploadRequest{
		ParentID: rootID,
		Name:     "degoo-cli-test.txt",
		Size:     int64(len(content)),
		MTime:    time.Now(),
		Open: func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(content)), nil
		},
	})
	if err != nil {
		t.Fatalf("UploadFile: %v", err)
	}
	t.Log("upload succeeded")
}

func TestDownloadFile(t *testing.T) {
	client := getTestClient(t)

	rootID, err := client.ResolveRemotePath("/")
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}

	// Find the test file uploaded in TestUploadSmallFile
	children, err := client.GetChildren(rootID)
	if err != nil {
		t.Fatalf("GetChildren: %v", err)
	}
	var testFile *api.FileInfo
	for i := range children {
		if children[i].Name == "degoo-cli-test.txt" {
			testFile = &children[i]
			break
		}
	}
	if testFile == nil {
		t.Skip("degoo-cli-test.txt not found — run TestUploadSmallFile first")
	}

	// getFileChildren5 does not populate DownloadURL; fetch it via getOverlay4.
	downloadURL := testFile.DownloadURL
	if downloadURL == "" {
		downloadURL, err = client.GetFileURL(testFile.ID)
		if err != nil {
			// Newly uploaded files may not be indexed yet; skip rather than fail.
			t.Skipf("GetFileURL: %v (file may not be indexed yet — retry later)", err)
		}
	}
	if downloadURL == "" {
		t.Skip("DownloadURL is empty — file may not be indexed yet")
	}

	var buf bytes.Buffer
	if err := client.DownloadFile(downloadURL, &buf); err != nil {
		t.Fatalf("DownloadFile: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("expected non-empty download data")
	}
	t.Logf("downloaded %d bytes", buf.Len())
}
