package oss

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	alioss "github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// Client wraps the Aliyun OSS bucket for file uploads.
type Client struct {
	bucket   *alioss.Bucket
	basePath string
	domain   string
}

// NewClient initializes an OSS client from environment variables.
func NewClient() (*Client, error) {
	accessKeyID := os.Getenv("OSS_ACCESS_KEY_ID")
	accessKeySecret := os.Getenv("OSS_ACCESS_KEY_SECRET")
	endpoint := os.Getenv("OSS_ENDPOINT")
	bucketName := os.Getenv("OSS_BUCKET_NAME")
	basePath := os.Getenv("OSS_BASE_PATH")
	domain := os.Getenv("OSS_DOMAIN")

	if accessKeyID == "" || accessKeySecret == "" || endpoint == "" || bucketName == "" {
		return nil, fmt.Errorf("OSS config incomplete: check OSS_ACCESS_KEY_ID, OSS_ACCESS_KEY_SECRET, OSS_ENDPOINT, OSS_BUCKET_NAME")
	}

	client, err := alioss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("create oss client: %w", err)
	}

	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("get oss bucket: %w", err)
	}

	return &Client{bucket: bucket, basePath: basePath, domain: domain}, nil
}

// UploadResult holds the result of a successful upload.
type UploadResult struct {
	URL string
	Key string
}

// Upload streams a reader to OSS under a date-based path.
func (c *Client) Upload(r io.Reader, filename string) (*UploadResult, error) {
	datePath := time.Now().Format("2006/01/02")
	objectKey := fmt.Sprintf("%s/%s/%s", c.basePath, datePath, filename)

	if err := c.bucket.PutObject(objectKey, r); err != nil {
		return nil, fmt.Errorf("oss put object: %w", err)
	}

	return &UploadResult{URL: fmt.Sprintf("%s/%s", c.domain, objectKey), Key: fmt.Sprintf("%s/%s", datePath, filename)}, nil
}

// Delete removes an object from OSS by its key (the path under basePath).
func (c *Client) Delete(key string) error {
	objectKey := fmt.Sprintf("%s/%s", c.basePath, key)
	if err := c.bucket.DeleteObject(objectKey); err != nil {
		return fmt.Errorf("oss delete object: %w", err)
	}
	return nil
}

// FullURL returns the complete URL for a relative key stored in the database.
// The relative key is the path under basePath (e.g. "2006/01/02/file.jpg").
func (c *Client) FullURL(relativePath string) string {
	if relativePath == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", strings.TrimRight(c.domain, "/"), strings.TrimRight(c.basePath, "/"), strings.TrimLeft(relativePath, "/"))
}

// FullURL is a package-level helper that resolves a relative OSS key to a
// complete URL using OSS_DOMAIN and OSS_BASE_PATH environment variables.
// This allows model/VO layers to build full URLs without holding a Client.
func FullURL(relativePath string) string {
	if relativePath == "" {
		return ""
	}
	domain := strings.TrimRight(os.Getenv("OSS_DOMAIN"), "/")
	basePath := strings.TrimRight(os.Getenv("OSS_BASE_PATH"), "/")
	relativePath = strings.TrimLeft(relativePath, "/")
	if basePath == "" {
		return fmt.Sprintf("%s/%s", domain, relativePath)
	}
	return fmt.Sprintf("%s/%s/%s", domain, basePath, relativePath)
}
