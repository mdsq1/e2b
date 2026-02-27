package e2b

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// readDockerignore reads and parses a .dockerignore file from the given directory.
// Returns empty slice if the file doesn't exist.
func readDockerignore(contextPath string) []string {
	dockerignorePath := filepath.Join(contextPath, ".dockerignore")
	data, err := os.ReadFile(dockerignorePath)
	if err != nil {
		return nil
	}

	var patterns []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns
}

// shouldIgnore checks whether a relative path matches any of the ignore patterns.
// Supports simple globs via filepath.Match and recursive ** patterns.
func shouldIgnore(relPath string, ignorePatterns []string) bool {
	for _, pattern := range ignorePatterns {
		// Handle ** recursive patterns
		if strings.Contains(pattern, "**") {
			// "**/foo" matches "foo" at any depth
			// "dir/**" matches anything under dir
			suffix := strings.TrimPrefix(pattern, "**/")
			if suffix != pattern {
				// Pattern was **/X — match X against basename or tail of path
				if matched, _ := filepath.Match(suffix, filepath.Base(relPath)); matched {
					return true
				}
				// Also try matching against each path suffix
				parts := strings.Split(relPath, string(filepath.Separator))
				for i := range parts {
					tail := strings.Join(parts[i:], string(filepath.Separator))
					if matched, _ := filepath.Match(suffix, tail); matched {
						return true
					}
				}
			}
			prefix := strings.TrimSuffix(pattern, "/**")
			if prefix != pattern {
				// Pattern was X/** — match if path starts with X/
				if strings.HasPrefix(relPath, prefix+string(filepath.Separator)) || relPath == prefix {
					return true
				}
			}
			continue
		}
		// Simple glob: match against full relative path
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
		// Also check basename match (e.g. "*.pyc" matches "dir/foo.pyc")
		if matched, _ := filepath.Match(pattern, filepath.Base(relPath)); matched {
			return true
		}
	}
	return false
}

// getAllFilesInPath returns all files under a source path within the context directory,
// excluding files matching ignore patterns. Results are sorted for deterministic hashing.
func getAllFilesInPath(src, contextPath string, ignorePatterns []string) ([]string, error) {
	fullPath := filepath.Join(contextPath, src)
	var files []string

	err := filepath.WalkDir(fullPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(contextPath, path)

		if shouldIgnore(relPath, ignorePatterns) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !d.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(files)
	return files, nil
}

// calculateFilesHash computes a deterministic hash for a source path
// relative to the context directory. Uses file content for accuracy.
func calculateFilesHash(src, dest, contextPath string, ignorePatterns []string, resolveSymlinks bool) (string, error) {
	h := sha256.New()

	// Hash the COPY instruction string (same as Python)
	fmt.Fprintf(h, "COPY %s %s", src, dest)

	files, err := getAllFilesInPath(src, contextPath, ignorePatterns)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no files found in %s", filepath.Join(contextPath, src))
	}

	for _, file := range files {
		relPath, _ := filepath.Rel(contextPath, file)
		h.Write([]byte(relPath))

		info, err := os.Lstat(file)
		if err != nil {
			return "", err
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			if !resolveSymlinks {
				// Hash symlink metadata + target
				fmt.Fprintf(h, "%d%d", info.Mode(), info.Size())
				target, _ := os.Readlink(file)
				h.Write([]byte(target))
				continue
			}
			// Follow symlink — get real file info
			info, err = os.Stat(file)
			if err != nil {
				return "", err
			}
		}

		// Hash stable metadata (mode, size)
		fmt.Fprintf(h, "%d%d", info.Mode(), info.Size())

		// Hash file content
		if info.Mode().IsRegular() {
			f, err := os.Open(file)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(h, f); err != nil {
				f.Close()
				return "", fmt.Errorf("hashing file %s: %w", file, err)
			}
			f.Close()
		}
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// fileUploadResponse is the API response for file upload link.
type fileUploadResponse struct {
	Present bool   `json:"present"`
	URL     string `json:"url"`
}

// getFileUploadLink gets a presigned upload URL for a files hash.
func (c *Client) getFileUploadLink(ctx context.Context, templateID, filesHash string) (*fileUploadResponse, error) {
	path := fmt.Sprintf("/templates/%s/files/%s", templateID, filesHash)
	var resp fileUploadResponse
	err := c.doRequest(ctx, "GET", path, nil, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// tarFileStream creates a tar.gz archive of a source path.
func tarFileStream(src, contextPath string, ignorePatterns []string, resolveSymlinks bool) (*bytes.Buffer, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	fullPath := filepath.Join(contextPath, src)

	err := filepath.WalkDir(fullPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(contextPath, path)

		if shouldIgnore(relPath, ignorePatterns) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return err
		}

		// Handle symlinks
		if info.Mode()&os.ModeSymlink != 0 && resolveSymlinks {
			info, err = os.Stat(path)
			if err != nil {
				return err
			}
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if !d.IsDir() && info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(tw, f)
			f.Close()
			if copyErr != nil {
				return copyErr
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return &buf, nil
}

// uploadFile uploads a tarred file to the presigned URL.
func (c *Client) uploadFile(ctx context.Context, url string, data *bytes.Buffer) error {
	req, err := http.NewRequestWithContext(ctx, "PUT", url, data)
	if err != nil {
		return &FileUploadError{SandboxError{Message: fmt.Sprintf("failed to create upload request: %v", err), Cause: err}}
	}
	req.Header.Set("Content-Type", "application/gzip")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return &FileUploadError{SandboxError{Message: fmt.Sprintf("file upload failed: %v", err), Cause: err}}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &FileUploadError{SandboxError{Message: fmt.Sprintf("file upload failed (HTTP %d): %s", resp.StatusCode, string(body))}}
	}
	return nil
}
