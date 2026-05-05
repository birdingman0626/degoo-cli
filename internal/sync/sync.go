package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"degoo-cli/internal/api"
	"degoo-cli/internal/logger"
)

// Stats tracks the outcome of an upload or download operation.
type Stats struct {
	Uploaded   int
	Downloaded int
	Skipped    int
	Failed     []string
	Bytes      int64
}

// ShouldTransfer returns true if the source is strictly newer than the destination.
// A zero destMtime means the destination doesn't exist — always transfer.
func ShouldTransfer(sourceMtime, destMtime time.Time) bool {
	if destMtime.IsZero() {
		return true
	}
	return sourceMtime.After(destMtime)
}

// WithRetry calls fn up to maxAttempts times with exponential backoff (1s, 2s, 4s…).
func WithRetry(maxAttempts int, fn func() error) error {
	var err error
	for i := 0; i < maxAttempts; i++ {
		err = fn()
		if err == nil {
			return nil
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(1<<uint(i)) * time.Second)
		}
	}
	return err
}

// Upload recursively uploads localPath into remotePath on Degoo.
func Upload(client *api.Client, log *logger.Logger, localPath, remotePath string) (*Stats, error) {
	stats := &Stats{}

	type fileEntry struct {
		localFull string
		localRel  string
		info      os.FileInfo
	}

	var files []fileEntry
	err := filepath.Walk(localPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(localPath, path)
		files = append(files, fileEntry{path, rel, info})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", localPath, err)
	}

	total := len(files)
	log.Info("Starting upload: %s → %s (%d files)", localPath, remotePath, total)

	for i, entry := range files {
		remoteDir := remotePath
		if relDir := filepath.Dir(entry.localRel); relDir != "." {
			remoteDir = remotePath + "/" + filepath.ToSlash(relDir)
		}

		parentID, err := client.EnsureRemotePath(remoteDir)
		if err != nil {
			log.Error("[%d/%d] Cannot create remote dir %s: %v", i+1, total, remoteDir, err)
			stats.Failed = append(stats.Failed, entry.localRel)
			continue
		}

		// Compare remote mtime to decide whether to upload.
		children, childErr := client.GetChildren(parentID)
		if childErr != nil {
			log.Warn("[%d/%d] Cannot list remote dir %s (will re-upload): %v", i+1, total, remoteDir, childErr)
		}
		var remoteMtime time.Time
		for _, child := range children {
			if child.Name == entry.info.Name() {
				remoteMtime = child.ModifiedTime
				break
			}
		}

		if !ShouldTransfer(entry.info.ModTime(), remoteMtime) {
			log.Info("[%d/%d] Skipping %s (remote is up to date)", i+1, total, entry.localRel)
			stats.Skipped++
			continue
		}

		log.Info("[%d/%d] Uploading %s (%d KB)", i+1, total, entry.localRel, entry.info.Size()/1024)

		localFull := entry.localFull
		openFn := func() (io.ReadCloser, error) { return os.Open(localFull) }

		var uploadErr error
		for attempt := 1; attempt <= 3; attempt++ {
			uploadErr = client.UploadFile(api.UploadRequest{
				ParentID: parentID,
				Name:     entry.info.Name(),
				Size:     entry.info.Size(),
				MTime:    entry.info.ModTime(),
				Open:     openFn,
			})
			if uploadErr == nil {
				break
			}
			if attempt < 3 {
				log.Warn("[%d/%d] Failed to upload %s (attempt %d/3), retrying...", i+1, total, entry.localRel, attempt)
				time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
			}
		}
		if uploadErr != nil {
			log.Error("[%d/%d] %s failed after 3 attempts: %v", i+1, total, entry.localRel, uploadErr)
			stats.Failed = append(stats.Failed, entry.localRel)
		} else {
			stats.Uploaded++
			stats.Bytes += entry.info.Size()
		}
	}

	return stats, nil
}

// Download recursively downloads remotePath from Degoo to localPath.
func Download(client *api.Client, log *logger.Logger, remotePath, localPath string) (*Stats, error) {
	stats := &Stats{}

	remoteID, err := client.ResolveRemotePath(remotePath)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", remotePath, err)
	}

	// BFS walk of remote tree to collect all files.
	type remoteEntry struct {
		file      api.FileInfo
		localDest string
	}

	type queueItem struct {
		id        string
		localBase string
	}
	queue := []queueItem{{remoteID, localPath}}
	var files []remoteEntry

	for len(queue) > 0 {
		head := queue[0]
		queue = queue[1:]

		children, err := client.GetChildren(head.id)
		if err != nil {
			return nil, fmt.Errorf("list %s: %w", head.id, err)
		}
		for _, child := range children {
			dest := filepath.Join(head.localBase, child.Name)
			if child.IsDirectory {
				queue = append(queue, queueItem{child.ID, dest})
			} else {
				files = append(files, remoteEntry{child, dest})
			}
		}
	}

	total := len(files)
	log.Info("Starting download: %s → %s (%d files)", remotePath, localPath, total)

	for i, entry := range files {
		var localMtime time.Time
		if st, err := os.Stat(entry.localDest); err == nil {
			localMtime = st.ModTime()
		}

		if !ShouldTransfer(entry.file.ModifiedTime, localMtime) {
			log.Info("[%d/%d] Skipping %s (local is up to date)", i+1, total, entry.file.Name)
			stats.Skipped++
			continue
		}

		log.Info("[%d/%d] Downloading %s (%d KB)", i+1, total, entry.file.Name, entry.file.Size/1024)

		if err := os.MkdirAll(filepath.Dir(entry.localDest), 0755); err != nil {
			log.Error("[%d/%d] Cannot create dir for %s: %v", i+1, total, entry.localDest, err)
			stats.Failed = append(stats.Failed, entry.file.Name)
			continue
		}

		var downloadErr error
		for attempt := 1; attempt <= 3; attempt++ {
			url, urlErr := client.GetFileURL(entry.file.ID)
			if urlErr != nil {
				downloadErr = fmt.Errorf("get URL: %w", urlErr)
			} else {
				var f *os.File
				f, downloadErr = os.Create(entry.localDest)
				if downloadErr == nil {
					downloadErr = client.DownloadFile(url, f)
					f.Close()
				}
			}
			if downloadErr == nil {
				break
			}
			if attempt < 3 {
				log.Warn("[%d/%d] Failed to download %s (attempt %d/3), retrying...", i+1, total, entry.file.Name, attempt)
				time.Sleep(time.Duration(1<<uint(attempt-1)) * time.Second)
			}
		}
		if downloadErr != nil {
			os.Remove(entry.localDest) // remove partial file to avoid skipping on next run
			log.Error("[%d/%d] %s failed after 3 attempts: %v", i+1, total, entry.file.Name, downloadErr)
			stats.Failed = append(stats.Failed, entry.file.Name)
		} else {
			stats.Downloaded++
			stats.Bytes += entry.file.Size
		}
	}

	return stats, nil
}
