package store

import (
	"crypto/sha512"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Avalanche-io/c4"
)

// testID returns a C4 ID computed from the given string.
func testID(s string) c4.ID {
	h := sha512.Sum512([]byte(s))
	var id c4.ID
	copy(id[:], h[:])
	return id
}

func TestS3KeyFormat(t *testing.T) {
	s := NewS3Store("mybucket", "c4/", "us-east-1", "", "AKID", "SECRET")
	id := testID("hello")
	key := s.objectKey(id)
	if !strings.HasPrefix(key, "c4/c4") {
		t.Fatalf("key should start with c4/c4, got %q", key)
	}
	if key != "c4/"+id.String() {
		t.Fatalf("key mismatch: want %q, got %q", "c4/"+id.String(), key)
	}
}

func TestS3URLConstructionAWS(t *testing.T) {
	s := NewS3Store("mybucket", "c4/", "us-west-2", "", "AKID", "SECRET")
	u := s.objectURL("c4/somekey")
	want := "https://mybucket.s3.us-west-2.amazonaws.com/c4/somekey"
	if u != want {
		t.Fatalf("AWS URL: want %q, got %q", want, u)
	}
}

func TestS3URLConstructionCustomEndpoint(t *testing.T) {
	s := NewS3Store("mybucket", "c4/", "us-east-1", "https://minio.local:9000", "AKID", "SECRET")
	u := s.objectURL("c4/somekey")
	want := "https://minio.local:9000/mybucket/c4/somekey"
	if u != want {
		t.Fatalf("custom endpoint URL: want %q, got %q", want, u)
	}
}

func TestS3URLConstructionHTTPEndpoint(t *testing.T) {
	s := NewS3Store("mybucket", "c4/", "us-east-1", "http://localhost:9000", "AKID", "SECRET")
	u := s.objectURL("c4/somekey")
	want := "http://localhost:9000/mybucket/c4/somekey"
	if u != want {
		t.Fatalf("HTTP endpoint URL: want %q, got %q", want, u)
	}
}

func TestS3HasNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Errorf("expected HEAD, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s := NewS3Store("testbucket", "c4/", "us-east-1", srv.URL, "AKID", "SECRET")
	id := testID("notfound")
	if s.Has(id) {
		t.Fatal("Has should return false for 404")
	}
}

func TestS3HasFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "HEAD" {
			t.Errorf("expected HEAD, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewS3Store("testbucket", "c4/", "us-east-1", srv.URL, "AKID", "SECRET")
	id := testID("found")
	if !s.Has(id) {
		t.Fatal("Has should return true for 200")
	}
}

func TestS3Open(t *testing.T) {
	content := "hello world from s3"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(content))
	}))
	defer srv.Close()

	s := NewS3Store("testbucket", "c4/", "us-east-1", srv.URL, "AKID", "SECRET")
	id := testID("hello")
	rc, err := s.Open(id)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != content {
		t.Fatalf("content mismatch: want %q, got %q", content, string(data))
	}
}

func TestS3Remove(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	s := NewS3Store("testbucket", "c4/", "us-east-1", srv.URL, "AKID", "SECRET")
	id := testID("removeme")
	if err := s.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
}

func TestS3Put(t *testing.T) {
	var receivedKey string
	var receivedBody string
	callCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		switch r.Method {
		case "HEAD":
			// Simulate not found so Put proceeds with upload.
			w.WriteHeader(http.StatusNotFound)
		case "PUT":
			receivedKey = r.URL.Path
			data, _ := io.ReadAll(r.Body)
			receivedBody = string(data)
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected method: %s", r.Method)
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	s := NewS3Store("testbucket", "data/", "us-east-1", srv.URL, "AKID", "SECRET")
	content := "test content for put"
	id, err := s.Put(strings.NewReader(content))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Verify the ID matches the expected C4 hash.
	expectedID := testID(content)
	if id != expectedID {
		t.Fatalf("ID mismatch: want %s, got %s", expectedID, id)
	}

	// Verify the key contains the prefix and ID.
	expectedPath := "/testbucket/data/" + id.String()
	if receivedKey != expectedPath {
		t.Fatalf("key mismatch: want %q, got %q", expectedPath, receivedKey)
	}

	// Verify content was uploaded.
	if receivedBody != content {
		t.Fatalf("body mismatch: want %q, got %q", content, receivedBody)
	}
}

func TestS3RetryOnSlowDown(t *testing.T) {
	var attempts int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := NewS3Store("testbucket", "c4/", "us-east-1", srv.URL, "AKID", "SECRET")
	id := testID("retry")
	if !s.Has(id) {
		t.Fatal("Has should return true after retries")
	}
	if n := atomic.LoadInt32(&attempts); n != 3 {
		t.Fatalf("expected 3 attempts, got %d", n)
	}
}

func TestS3SignRequest(t *testing.T) {
	s := NewS3Store("mybucket", "c4/", "us-east-1", "", "AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	req, _ := http.NewRequest("GET", "https://mybucket.s3.us-east-1.amazonaws.com/c4/testkey", nil)
	s.signRequest(req, "UNSIGNED-PAYLOAD")

	auth := req.Header.Get("Authorization")
	if auth == "" {
		t.Fatal("Authorization header missing")
	}
	if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
		t.Fatalf("Authorization should start with AWS4-HMAC-SHA256, got %q", auth)
	}
	if !strings.Contains(auth, "Credential=AKIAIOSFODNN7EXAMPLE/") {
		t.Fatalf("Authorization should contain access key, got %q", auth)
	}
	if !strings.Contains(auth, "/us-east-1/s3/aws4_request") {
		t.Fatalf("Authorization should contain scope, got %q", auth)
	}
	if !strings.Contains(auth, "SignedHeaders=") {
		t.Fatalf("Authorization should contain SignedHeaders, got %q", auth)
	}
	if !strings.Contains(auth, "Signature=") {
		t.Fatalf("Authorization should contain Signature, got %q", auth)
	}

	// Verify required headers are set.
	amzDate := req.Header.Get("x-amz-date")
	if amzDate == "" {
		t.Fatal("x-amz-date header missing")
	}
	if len(amzDate) != 16 || amzDate[8] != 'T' || amzDate[15] != 'Z' {
		t.Fatalf("x-amz-date format wrong: %q", amzDate)
	}

	contentSHA := req.Header.Get("x-amz-content-sha256")
	if contentSHA != "UNSIGNED-PAYLOAD" {
		t.Fatalf("x-amz-content-sha256: want UNSIGNED-PAYLOAD, got %q", contentSHA)
	}
}

func TestS3CanonicalRequest(t *testing.T) {
	// Test that uriEncodePath handles various path characters.
	tests := []struct {
		input string
		want  string
	}{
		{"/", "/"},
		{"/c4/testkey", "/c4/testkey"},
		{"/bucket/key with space", "/bucket/key%20with%20space"},
		{"/bucket/key+plus", "/bucket/key%2Bplus"},
	}
	for _, tt := range tests {
		got := uriEncodePath(tt.input)
		if got != tt.want {
			t.Errorf("uriEncodePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestS3URIEncode(t *testing.T) {
	tests := []struct {
		input       string
		encodeSlash bool
		want        string
	}{
		{"simple", false, "simple"},
		{"with space", false, "with%20space"},
		{"a/b/c", false, "a/b/c"},
		{"a/b/c", true, "a%2Fb%2Fc"},
		{"keep-these_chars.ok~", false, "keep-these_chars.ok~"},
		{"special!@#$", false, "special%21%40%23%24"},
	}
	for _, tt := range tests {
		got := uriEncode(tt.input, tt.encodeSlash)
		if got != tt.want {
			t.Errorf("uriEncode(%q, %v) = %q, want %q", tt.input, tt.encodeSlash, got, tt.want)
		}
	}
}

func TestS3ParseError(t *testing.T) {
	xml := `<Error><Code>NoSuchKey</Code><Message>The specified key does not exist.</Message></Error>`
	msg := parseS3Error([]byte(xml), 404)
	if msg != "NoSuchKey: The specified key does not exist." {
		t.Fatalf("parseS3Error: got %q", msg)
	}

	// Fallback on invalid XML.
	msg = parseS3Error([]byte("not xml"), 500)
	if msg != "HTTP 500" {
		t.Fatalf("parseS3Error fallback: got %q", msg)
	}
}

func TestS3StoreInterfaceCompliance(t *testing.T) {
	// Verify S3Store implements Store at compile time.
	var _ Store = (*S3Store)(nil)
}
