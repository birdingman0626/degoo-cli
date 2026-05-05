package api

import (
	"encoding/base64"
	"testing"
	"time"
)

func TestDegooChecksum(t *testing.T) {
	data := []byte("hello world")
	cs := degooChecksum(data)
	if cs == "" {
		t.Fatal("expected non-empty checksum")
	}
	// Decode and verify structure: [10, 20, ...20 bytes..., 16, 0] = 24 bytes
	decoded, err := base64.StdEncoding.DecodeString(cs)
	if err != nil {
		t.Fatalf("checksum not valid base64: %v", err)
	}
	if len(decoded) != 24 {
		t.Errorf("expected 24 decoded bytes, got %d", len(decoded))
	}
	if decoded[0] != 10 || decoded[1] != 20 || decoded[22] != 16 || decoded[23] != 0 {
		t.Errorf("checksum structure wrong: %v", decoded)
	}
}

func TestSplitPath(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"/", nil},
		{"", nil},
		{"/a/b", []string{"a", "b"}},
		{"a/b", []string{"a", "b"}},
		{"/My Files/Photos", []string{"My Files", "Photos"}},
	}
	for _, tt := range tests {
		got := splitPath(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("splitPath(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("splitPath(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestToFileInfo(t *testing.T) {
	tests := []struct {
		name    string
		input   rawNode
		wantDir bool
		wantID  string
	}{
		{
			name:    "folder category 4",
			input:   rawNode{ID: "123", Name: "Photos", Category: 4, Size: "0", LastModificationTime: "0"},
			wantDir: true,
			wantID:  "123",
		},
		{
			name:    "device root category 1",
			input:   rawNode{ID: "456", Name: "My Files", Category: 1, Size: "0", LastModificationTime: "0"},
			wantDir: true,
			wantID:  "456",
		},
		{
			name:    "file category 5",
			input:   rawNode{ID: "789", Name: "photo.jpg", Category: 5, Size: "1024", LastModificationTime: "1700000000000"},
			wantDir: false,
			wantID:  "789",
		},
		{
			name:    "empty ID falls back to MetadataID",
			input:   rawNode{ID: "", MetadataID: "meta99", Name: "file.txt", Category: 5, Size: "0", LastModificationTime: "0"},
			wantDir: false,
			wantID:  "meta99",
		},
		{
			name:    "valid mtime parses correctly",
			input:   rawNode{ID: "1", Name: "f", Category: 5, Size: "512", LastModificationTime: "1700000000000"},
			wantDir: false,
			wantID:  "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fi := toFileInfo(tt.input)
			if fi.IsDirectory != tt.wantDir {
				t.Errorf("IsDirectory = %v, want %v", fi.IsDirectory, tt.wantDir)
			}
			if fi.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", fi.ID, tt.wantID)
			}
		})
	}

	// Verify mtime parsing
	t.Run("mtime is correct epoch ms", func(t *testing.T) {
		fi := toFileInfo(rawNode{ID: "1", Category: 5, Size: "0", LastModificationTime: "1700000000000"})
		expected := time.UnixMilli(1700000000000)
		if !fi.ModifiedTime.Equal(expected) {
			t.Errorf("ModifiedTime = %v, want %v", fi.ModifiedTime, expected)
		}
	})
}
