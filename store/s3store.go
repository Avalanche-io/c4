package store

import (
	"bytes"
	"crypto/sha512"
	"encoding/xml"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Avalanche-io/c4"
)

const (
	// multipartThreshold is the size above which uploads use multipart.
	multipartThreshold = 8 * 1024 * 1024 // 8 MB
	// partSize is the size of each multipart upload part.
	partSize = 8 * 1024 * 1024 // 8 MB
	// maxRetries is the maximum number of retries for transient errors.
	maxRetries = 5
	// initialBackoff is the starting delay for exponential backoff.
	initialBackoff = 100 * time.Millisecond
	// maxConcurrentParts limits the number of concurrent part uploads.
	maxConcurrentParts = 4
)

// Verify S3Store satisfies the Store interface at compile time.
var _ Store = (*S3Store)(nil)

// S3Store is a content-addressed store backed by an S3-compatible object
// storage service. It uses only the Go standard library.
type S3Store struct {
	client    *http.Client
	bucket    string
	prefix    string // key prefix for all objects (e.g., "c4/")
	region    string
	endpoint  string // custom endpoint for non-AWS S3
	accessKey string
	secretKey string
	keyCache  signingKeyCache
}

// completedPart holds the result of uploading a single multipart part.
type completedPart struct {
	num  int
	etag string
	err  error
}

// NewS3Store creates a new S3Store. For AWS S3, leave endpoint empty.
// For S3-compatible services (MinIO, Backblaze B2, Wasabi, Ceph), set
// endpoint to the service URL (e.g., "s3.us-west-001.backblazeb2.com").
func NewS3Store(bucket, prefix, region, endpoint, accessKey, secretKey string) *S3Store {
	transport := &http.Transport{
		MaxIdleConnsPerHost: 10,
	}
	return &S3Store{
		client: &http.Client{
			Transport: transport,
			Timeout:   5 * time.Minute,
		},
		bucket:    bucket,
		prefix:    prefix,
		region:    region,
		endpoint:  endpoint,
		accessKey: accessKey,
		secretKey: secretKey,
	}
}

// objectKey returns the S3 object key for the given C4 ID.
func (s *S3Store) objectKey(id c4.ID) string {
	return s.prefix + id.String()
}

// objectURL returns the full URL for the given object key.
// For custom endpoints, path-style URLs are used since many S3-compatible
// services do not support virtual-hosted style. For AWS, virtual-hosted
// style is used.
func (s *S3Store) objectURL(key string) string {
	if s.endpoint != "" {
		// Path-style: {scheme}://{endpoint}/{bucket}/{key}
		scheme := "https"
		ep := s.endpoint
		if strings.HasPrefix(ep, "http://") {
			scheme = "http"
			ep = strings.TrimPrefix(ep, "http://")
		} else {
			ep = strings.TrimPrefix(ep, "https://")
		}
		ep = strings.TrimSuffix(ep, "/")
		return scheme + "://" + ep + "/" + s.bucket + "/" + key
	}
	// AWS virtual-hosted style: https://{bucket}.s3.{region}.amazonaws.com/{key}
	return "https://" + s.bucket + ".s3." + s.region + ".amazonaws.com/" + key
}

// bucketURL returns the URL for an object key with query parameters.
func (s *S3Store) bucketURL(key string, params map[string]string) string {
	base := s.objectURL(key)
	if len(params) == 0 {
		return base
	}
	vals := url.Values{}
	for k, v := range params {
		vals.Set(k, v)
	}
	return base + "?" + vals.Encode()
}

// Has reports whether the store contains content for the given ID.
func (s *S3Store) Has(id c4.ID) bool {
	key := s.objectKey(id)
	reqURL := s.objectURL(key)

	req, err := http.NewRequest("HEAD", reqURL, nil)
	if err != nil {
		return false
	}
	s.signRequest(req, "UNSIGNED-PAYLOAD")

	resp, err := s.doWithRetry(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Open opens the content for reading.
func (s *S3Store) Open(id c4.ID) (io.ReadCloser, error) {
	key := s.objectKey(id)
	reqURL := s.objectURL(key)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("s3 open: %w", err)
	}
	s.signRequest(req, "UNSIGNED-PAYLOAD")

	resp, err := s.doWithRetry(req)
	if err != nil {
		return nil, fmt.Errorf("s3 open: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("s3 open %s: %s", id, parseS3Error(body, resp.StatusCode))
	}
	return resp.Body, nil
}

// Create returns a writer that buffers content to a temp file and uploads
// to S3 on Close. The caller must know the C4 ID in advance.
func (s *S3Store) Create(id c4.ID) (io.WriteCloser, error) {
	tmp, err := os.CreateTemp("", "c4-s3-create-*")
	if err != nil {
		return nil, fmt.Errorf("s3 create temp: %w", err)
	}
	return &s3Writer{
		store: s,
		id:    id,
		tmp:   tmp,
	}, nil
}

// Put reads all content from r, computes its C4 ID, stores it, and returns
// the ID. If the content already exists the upload is skipped.
func (s *S3Store) Put(r io.Reader) (c4.ID, error) {
	tmp, err := os.CreateTemp("", "c4-s3-put-*")
	if err != nil {
		return c4.ID{}, fmt.Errorf("s3 put temp: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	h := sha512.New()
	w := io.MultiWriter(tmp, h)
	if _, err := io.Copy(w, r); err != nil {
		tmp.Close()
		return c4.ID{}, fmt.Errorf("s3 put copy: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return c4.ID{}, fmt.Errorf("s3 put close temp: %w", err)
	}

	var id c4.ID
	copy(id[:], h.Sum(nil))

	if s.Has(id) {
		return id, nil
	}

	if err := s.uploadFile(tmpName, s.objectKey(id)); err != nil {
		return c4.ID{}, fmt.Errorf("s3 put upload: %w", err)
	}
	return id, nil
}

// Remove deletes the content for the given ID.
func (s *S3Store) Remove(id c4.ID) error {
	key := s.objectKey(id)
	reqURL := s.objectURL(key)

	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("s3 remove: %w", err)
	}
	s.signRequest(req, "UNSIGNED-PAYLOAD")

	resp, err := s.doWithRetry(req)
	if err != nil {
		return fmt.Errorf("s3 remove: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("s3 remove %s: unexpected status %d", id, resp.StatusCode)
	}
	return nil
}

// uploadFile uploads a local file to the given S3 key. Files larger than
// multipartThreshold use multipart upload.
func (s *S3Store) uploadFile(path, key string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() > multipartThreshold {
		return s.multipartUpload(path, key, info.Size())
	}
	return s.singleUpload(path, key)
}

// singleUpload performs a simple PUT upload.
func (s *S3Store) singleUpload(path, key string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	payloadHash := hashSHA256(data)
	reqURL := s.objectURL(key)

	req, err := http.NewRequest("PUT", reqURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", strconv.Itoa(len(data)))
	s.signRequest(req, payloadHash)

	resp, err := s.doWithRetry(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("s3 put: unexpected status %d", resp.StatusCode)
	}
	return nil
}

// multipartUpload performs a multipart upload for large files.
func (s *S3Store) multipartUpload(path, key string, size int64) error {
	uploadID, err := s.initiateMultipart(key)
	if err != nil {
		return fmt.Errorf("initiate multipart: %w", err)
	}

	numParts := int((size + partSize - 1) / partSize)
	parts := make([]completedPart, numParts)

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxConcurrentParts)

	for i := 0; i < numParts; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(partNum int) {
			defer wg.Done()
			defer func() { <-sem }()

			offset := int64(partNum) * partSize
			length := partSize
			if offset+int64(length) > size {
				length = int(size - offset)
			}

			f, ferr := os.Open(path)
			if ferr != nil {
				parts[partNum] = completedPart{num: partNum + 1, err: ferr}
				return
			}
			defer f.Close()

			if _, ferr = f.Seek(offset, io.SeekStart); ferr != nil {
				parts[partNum] = completedPart{num: partNum + 1, err: ferr}
				return
			}

			data := make([]byte, length)
			if _, ferr = io.ReadFull(f, data); ferr != nil {
				parts[partNum] = completedPart{num: partNum + 1, err: ferr}
				return
			}

			etag, ferr := s.uploadPart(key, uploadID, partNum+1, data)
			parts[partNum] = completedPart{num: partNum + 1, etag: etag, err: ferr}
		}(i)
	}
	wg.Wait()

	for _, p := range parts {
		if p.err != nil {
			s.abortMultipart(key, uploadID)
			return fmt.Errorf("upload part %d: %w", p.num, p.err)
		}
	}

	return s.completeMultipart(key, uploadID, parts)
}

// initiateMultipart starts a multipart upload and returns the upload ID.
func (s *S3Store) initiateMultipart(key string) (string, error) {
	reqURL := s.bucketURL(key, map[string]string{"uploads": ""})

	req, err := http.NewRequest("POST", reqURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	s.signRequest(req, hashSHA256(nil))

	resp, err := s.doWithRetry(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("initiate multipart: %s", parseS3Error(body, resp.StatusCode))
	}

	var result struct {
		UploadId string `xml:"UploadId"`
	}
	if err := xml.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse multipart init response: %w", err)
	}
	return result.UploadId, nil
}

// uploadPart uploads a single part and returns its ETag.
func (s *S3Store) uploadPart(key, uploadID string, partNum int, data []byte) (string, error) {
	params := map[string]string{
		"partNumber": strconv.Itoa(partNum),
		"uploadId":   uploadID,
	}
	reqURL := s.bucketURL(key, params)
	payloadHash := hashSHA256(data)

	req, err := http.NewRequest("PUT", reqURL, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", strconv.Itoa(len(data)))
	s.signRequest(req, payloadHash)

	resp, err := s.doWithRetry(req)
	if err != nil {
		return "", err
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("upload part %d: unexpected status %d", partNum, resp.StatusCode)
	}
	return resp.Header.Get("ETag"), nil
}

// completeMultipart finishes the multipart upload by sending the part list.
func (s *S3Store) completeMultipart(key, uploadID string, parts []completedPart) error {
	var buf bytes.Buffer
	buf.WriteString("<CompleteMultipartUpload>")
	for _, p := range parts {
		buf.WriteString("<Part><PartNumber>")
		buf.WriteString(strconv.Itoa(p.num))
		buf.WriteString("</PartNumber><ETag>")
		buf.WriteString(p.etag)
		buf.WriteString("</ETag></Part>")
	}
	buf.WriteString("</CompleteMultipartUpload>")

	data := buf.Bytes()
	payloadHash := hashSHA256(data)
	params := map[string]string{"uploadId": uploadID}
	reqURL := s.bucketURL(key, params)

	req, err := http.NewRequest("POST", reqURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Content-Length", strconv.Itoa(len(data)))
	s.signRequest(req, payloadHash)

	resp, err := s.doWithRetry(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("complete multipart: %s", parseS3Error(body, resp.StatusCode))
	}
	return nil
}

// abortMultipart cancels a multipart upload.
func (s *S3Store) abortMultipart(key, uploadID string) {
	params := map[string]string{"uploadId": uploadID}
	reqURL := s.bucketURL(key, params)

	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return
	}
	s.signRequest(req, "UNSIGNED-PAYLOAD")

	resp, err := s.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

// doWithRetry executes an HTTP request with exponential backoff retry
// on transient errors (503 SlowDown, 500 InternalError).
func (s *S3Store) doWithRetry(req *http.Request) (*http.Response, error) {
	backoff := initialBackoff
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			jitter := time.Duration(float64(backoff) * (0.75 + rand.Float64()*0.5))
			time.Sleep(jitter)
			backoff *= 2

			// Re-sign on retry (timestamp changes).
			payloadHash := req.Header.Get("x-amz-content-sha256")
			s.signRequest(req, payloadHash)
		}

		resp, err := s.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusInternalServerError {
			resp.Body.Close()
			lastErr = fmt.Errorf("s3: status %d", resp.StatusCode)
			continue
		}

		return resp, nil
	}
	return nil, fmt.Errorf("s3: max retries exceeded: %w", lastErr)
}

// s3Writer buffers writes to a temp file and uploads to S3 on Close.
type s3Writer struct {
	store *S3Store
	id    c4.ID
	tmp   *os.File
}

func (w *s3Writer) Write(b []byte) (int, error) {
	return w.tmp.Write(b)
}

func (w *s3Writer) Close() error {
	tmpName := w.tmp.Name()
	defer os.Remove(tmpName)

	if err := w.tmp.Close(); err != nil {
		return fmt.Errorf("s3 writer close temp: %w", err)
	}

	key := w.store.objectKey(w.id)
	if err := w.store.uploadFile(tmpName, key); err != nil {
		return fmt.Errorf("s3 writer upload: %w", err)
	}
	return nil
}

// s3Error represents an S3 XML error response.
type s3Error struct {
	Code    string `xml:"Code"`
	Message string `xml:"Message"`
}

// parseS3Error extracts a human-readable error from an S3 XML error
// response body. Falls back to the HTTP status code if parsing fails.
func parseS3Error(body []byte, statusCode int) string {
	var e s3Error
	if xml.Unmarshal(body, &e) == nil && e.Code != "" {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("HTTP %d", statusCode)
}
