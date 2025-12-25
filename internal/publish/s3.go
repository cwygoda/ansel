package publish

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ContentTypes maps file extensions to MIME types.
var contentTypes = map[string]string{
	".html": "text/html; charset=utf-8",
	".htm":  "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "application/javascript",
	".mjs":  "application/javascript",
	".json": "application/json",
	".xml":  "application/xml",

	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".avif": "image/avif",
	".svg":  "image/svg+xml",
	".ico":  "image/x-icon",

	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
	".otf":   "font/otf",
	".eot":   "application/vnd.ms-fontobject",

	".pdf":  "application/pdf",
	".zip":  "application/zip",
	".txt":  "text/plain; charset=utf-8",
	".md":   "text/markdown; charset=utf-8",
	".yaml": "text/yaml; charset=utf-8",
	".yml":  "text/yaml; charset=utf-8",

	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mp3":  "audio/mpeg",
	".wav":  "audio/wav",
	".ogg":  "audio/ogg",
}

func getContentType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ct, ok := contentTypes[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

// SyncDirectory uploads all files from buildDir to the S3 bucket.
// Only uploads files that have changed (based on ETag/MD5 comparison).
func (c *AWSClients) SyncDirectory(ctx context.Context, bucket, buildDir string) error {
	// Get existing objects
	existing, err := c.listObjects(ctx, bucket)
	if err != nil {
		return err
	}

	// Walk the build directory and collect files to upload
	var files []string
	err = filepath.WalkDir(buildDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk build directory: %w", err)
	}

	if len(files) == 0 {
		return fmt.Errorf("no files found in %s", buildDir)
	}

	fmt.Fprintf(os.Stderr, "Syncing %d files to s3://%s\n", len(files), bucket)

	uploaded := 0
	skipped := 0

	for _, path := range files {
		relPath, err := filepath.Rel(buildDir, path)
		if err != nil {
			return err
		}
		// Use forward slashes for S3 keys
		key := filepath.ToSlash(relPath)

		// Check if file needs uploading
		localMD5, err := fileMD5(path)
		if err != nil {
			return fmt.Errorf("failed to compute MD5 for %s: %w", path, err)
		}

		if etag, ok := existing[key]; ok {
			// Compare ETag (without quotes) to local MD5
			etag = strings.Trim(etag, "\"")
			if etag == localMD5 {
				skipped++
				continue
			}
		}

		// Upload the file
		if err := c.uploadFile(ctx, bucket, key, path); err != nil {
			return fmt.Errorf("failed to upload %s: %w", path, err)
		}
		fmt.Fprintf(os.Stderr, "  Uploaded: %s\n", key)
		uploaded++
	}

	fmt.Fprintf(os.Stderr, "Sync complete: %d uploaded, %d unchanged\n", uploaded, skipped)
	return nil
}

func (c *AWSClients) listObjects(ctx context.Context, bucket string) (map[string]string, error) {
	objects := make(map[string]string)

	paginator := s3.NewListObjectsV2Paginator(c.S3, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			// Bucket might not exist yet or be empty
			return objects, nil
		}
		for _, obj := range page.Contents {
			if obj.Key != nil && obj.ETag != nil {
				objects[*obj.Key] = *obj.ETag
			}
		}
	}

	return objects, nil
}

func (c *AWSClients) uploadFile(ctx context.Context, bucket, key, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	contentType := getContentType(path)

	_, err = c.S3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	return err
}

func fileMD5(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}
