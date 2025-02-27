//go:build !js

package bldr_util_filehash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAddHashToFilename(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		hash     string
		want     string
	}{
		{
			name:     "simple js file",
			filename: "example.js",
			hash:     "abc123",
			want:     "example-abc123.js",
		},
		{
			name:     "file without extension",
			filename: "noextension",
			hash:     "def456",
			want:     "noextension-def456",
		},
		{
			name:     "file with multiple dots",
			filename: "multi.part.file.js",
			hash:     "ghi789",
			want:     "multi.part.file-ghi789.js",
		},
		{
			name:     "file with path",
			filename: "path/to/file.js",
			hash:     "jkl012",
			want:     "path/to/file-jkl012.js",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AddHashToFilename(tt.filename, tt.hash)
			if got != tt.want {
				t.Errorf("AddHashToFilename() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHashFileWithBlake3(t *testing.T) {
	// Create a temporary file
	tempDir := t.TempDir()
	tempFile := filepath.Join(tempDir, "test.txt")

	// Test with a simple file
	content := []byte("This is a test file for hashing")
	if err := os.WriteFile(tempFile, content, 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	hash, err := HashFileWithBlake3(tempFile)
	if err != nil {
		t.Fatalf("HashFileWithBlake3() error = %v", err)
	}

	// Verify hash is not empty and has the expected format (8 lowercase base32 chars)
	if len(hash) != 8 {
		t.Errorf("HashFileWithBlake3() returned hash of length %d, want 8", len(hash))
	}
	for _, c := range hash {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyz234567", c) {
			t.Errorf("HashFileWithBlake3() returned hash with invalid character: %c", c)
		}
	}

	// Test with a non-existent file
	_, err = HashFileWithBlake3(filepath.Join(tempDir, "nonexistent.txt"))
	if err == nil {
		t.Error("HashFileWithBlake3() expected error for non-existent file, got nil")
	}

	// Test consistency - same file should produce same hash
	hash2, err := HashFileWithBlake3(tempFile)
	if err != nil {
		t.Fatalf("HashFileWithBlake3() second call error = %v", err)
	}
	if hash != hash2 {
		t.Errorf("HashFileWithBlake3() not consistent: got %v then %v", hash, hash2)
	}

	// Test different content produces different hash
	differentContent := []byte("This is different content")
	differentFile := filepath.Join(tempDir, "different.txt")
	if err := os.WriteFile(differentFile, differentContent, 0o644); err != nil {
		t.Fatalf("Failed to create different test file: %v", err)
	}

	differentHash, err := HashFileWithBlake3(differentFile)
	if err != nil {
		t.Fatalf("HashFileWithBlake3() error for different file = %v", err)
	}
	if hash == differentHash {
		t.Errorf("HashFileWithBlake3() should produce different hashes for different content")
	}
}

func TestUpdateSourceMapReference(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name            string
		initialContent  string
		newMapFilename  string
		expectedContent string
	}{
		{
			name:            "no existing sourcemap",
			initialContent:  "console.log('hello');\n",
			newMapFilename:  "file.js.map",
			expectedContent: "console.log('hello');\n//# sourceMappingURL=file.js.map",
		},
		{
			name:            "replace existing sourcemap",
			initialContent:  "console.log('hello');\n//# sourceMappingURL=old.js.map",
			newMapFilename:  "new.js.map",
			expectedContent: "console.log('hello');\n//# sourceMappingURL=new.js.map",
		},
		{
			name:            "sourcemap in the middle",
			initialContent:  "line1\n//# sourceMappingURL=old.js.map\nline3",
			newMapFilename:  "new.js.map",
			expectedContent: "line1\nline3\n//# sourceMappingURL=new.js.map",
		},
		{
			name:            "sourcemap with whitespace",
			initialContent:  "code\n  //# sourceMappingURL=old.js.map\nmore code",
			newMapFilename:  "new.js.map",
			expectedContent: "code\nmore code\n//# sourceMappingURL=new.js.map",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file with the initial content
			tempFile := filepath.Join(tempDir, tt.name+".js")
			if err := os.WriteFile(tempFile, []byte(tt.initialContent), 0o644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Update the source map reference
			err := UpdateSourceMapReference(tempFile, tt.newMapFilename)
			if err != nil {
				t.Fatalf("UpdateSourceMapReference() error = %v", err)
			}

			// Read the updated file
			updatedContent, err := os.ReadFile(tempFile)
			if err != nil {
				t.Fatalf("Failed to read updated file: %v", err)
			}

			// Check if the content matches the expected content
			if string(updatedContent) != tt.expectedContent {
				t.Errorf("UpdateSourceMapReference() resulted in content:\n%s\nwant:\n%s",
					string(updatedContent), tt.expectedContent)
			}
		})
	}

	// Test with a non-existent file
	err := UpdateSourceMapReference(filepath.Join(tempDir, "nonexistent.js"), "map.js.map")
	if err == nil {
		t.Error("UpdateSourceMapReference() expected error for non-existent file, got nil")
	}
}
