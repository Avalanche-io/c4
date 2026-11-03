package store

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeMultipart is a bucket for multipart uploads with knobs for the
// failures S3 can produce: a lost reply to the start, a completion that
// answers 200 with an error body or a cut-off body or a 500, and an
// abort that fails. Ported from the operator's s3review tests.
type fakeMultipart struct {
	mu        sync.Mutex
	objects   map[string][]byte
	uploads   map[string]map[int][]byte
	initiates int32 // atomic
	aborts    int32 // atomic

	loseInitiate bool   // drop the connection on the start request
	complete     string // "ok", "error-body", "cutoff", "500"
	abortFails   bool
}

func (f *fakeMultipart) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	body, _ := io.ReadAll(r.Body)
	switch {
	case r.Method == http.MethodPost && q.Has("uploads"):
		atomic.AddInt32(&f.initiates, 1)
		if f.loseInitiate {
			hj, _ := w.(http.Hijacker)
			c, _, _ := hj.Hijack()
			c.Close()
			return
		}
		f.mu.Lock()
		id := fmt.Sprintf("up%d", len(f.uploads)+1)
		f.uploads[id] = map[int][]byte{}
		f.mu.Unlock()
		fmt.Fprintf(w, "<InitiateMultipartUploadResult><UploadId>%s</UploadId></InitiateMultipartUploadResult>", id)
	case r.Method == http.MethodPut && q.Get("uploadId") != "":
		var n int
		fmt.Sscan(q.Get("partNumber"), &n)
		f.mu.Lock()
		if up, ok := f.uploads[q.Get("uploadId")]; ok {
			up[n] = body
		}
		f.mu.Unlock()
		w.Header().Set("ETag", fmt.Sprintf(`"etag-%d"`, n))
	case r.Method == http.MethodPost && q.Get("uploadId") != "":
		switch f.complete {
		case "error-body":
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "<Error><Code>InternalError</Code><Message>We encountered an internal error. Please try again.</Message></Error>")
		case "cutoff":
			w.WriteHeader(http.StatusOK)
			io.WriteString(w, "<CompleteMultipartUploadResul")
			w.(http.Flusher).Flush()
			hj, _ := w.(http.Hijacker)
			c, _, _ := hj.Hijack()
			c.Close()
		case "500":
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, "<Error><Code>InternalError</Code><Message>no</Message></Error>")
		default:
			f.mu.Lock()
			parts := f.uploads[q.Get("uploadId")]
			var all []byte
			for i := 1; i <= len(parts); i++ {
				all = append(all, parts[i]...)
			}
			f.objects[key] = all
			f.mu.Unlock()
			io.WriteString(w, "<CompleteMultipartUploadResult><Key>"+key+"</Key></CompleteMultipartUploadResult>")
		}
	case r.Method == http.MethodDelete && q.Get("uploadId") != "":
		atomic.AddInt32(&f.aborts, 1)
		if f.abortFails {
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, "<Error><Code>AccessDenied</Code><Message>no</Message></Error>")
			return
		}
		f.mu.Lock()
		delete(f.uploads, q.Get("uploadId"))
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodHead:
		f.mu.Lock()
		b, ok := f.objects[key]
		f.mu.Unlock()
		if ok {
			w.Header().Set("Content-Length", fmt.Sprint(len(b)))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func multipartStore(t *testing.T, f *fakeMultipart) *S3Store {
	t.Helper()
	if f.objects == nil {
		f.objects = map[string][]byte{}
	}
	if f.uploads == nil {
		f.uploads = map[string]map[int][]byte{}
	}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return NewS3Store("bucket", "p/", "us-east-1", srv.URL, "AK", "SK")
}

// bigObject is larger than one part, so Put goes multipart.
func bigObject() []byte {
	b := make([]byte, multipartThreshold+multipartThreshold/2)
	for i := range b {
		b[i] = byte(i)
	}
	return b
}

// A completion that answers 200 and then reports an error in its body is
// a failure, and the upload is aborted.
func TestMultipartCompletionErrorBodyFails(t *testing.T) {
	f := &fakeMultipart{complete: "error-body"}
	s := multipartStore(t, f)
	_, err := s.Put(bytes.NewReader(bigObject()))
	if err == nil || !strings.Contains(err.Error(), "InternalError") {
		t.Fatalf("a 200 with an error body was taken as success: %v", err)
	}
	if atomic.LoadInt32(&f.aborts) != 1 {
		t.Fatalf("the upload was not aborted (%d aborts)", atomic.LoadInt32(&f.aborts))
	}
}

// A completion whose reply is cut off is a failure, and the upload is
// aborted.
func TestMultipartCompletionCutOffFails(t *testing.T) {
	f := &fakeMultipart{complete: "cutoff"}
	s := multipartStore(t, f)
	_, err := s.Put(bytes.NewReader(bigObject()))
	if err == nil || !strings.Contains(err.Error(), "cut off") {
		t.Fatalf("a cut-off completion was taken as success: %v", err)
	}
	if atomic.LoadInt32(&f.aborts) != 1 {
		t.Fatalf("the upload was not aborted (%d aborts)", atomic.LoadInt32(&f.aborts))
	}
}

// A completion that fails with a status aborts the upload; an abort that
// fails names the upload left open.
func TestMultipartFailedCompletionAbortsAndAFailedAbortIsNamed(t *testing.T) {
	f := &fakeMultipart{complete: "500", abortFails: true}
	s := multipartStore(t, f)
	_, err := s.Put(bytes.NewReader(bigObject()))
	if err == nil || !strings.Contains(err.Error(), "left open") || !strings.Contains(err.Error(), "up1") {
		t.Fatalf("a failed abort should name the upload left open: %v", err)
	}
	if atomic.LoadInt32(&f.aborts) == 0 {
		t.Fatal("no abort was attempted")
	}
}

// Starting an upload is not idempotent: a lost reply is not retried.
func TestMultipartStartIsNotRetriedAfterALostReply(t *testing.T) {
	f := &fakeMultipart{loseInitiate: true}
	s := multipartStore(t, f)
	_, err := s.Put(bytes.NewReader(bigObject()))
	if err == nil || !strings.Contains(err.Error(), "AbortIncompleteMultipartUpload") {
		t.Fatalf("a lost start reply should fail and say what to do: %v", err)
	}
	if n := atomic.LoadInt32(&f.initiates); n != 1 {
		t.Fatalf("the start was sent %d times, want 1", n)
	}
}

// The happy path still works: parts go up and the object is joined.
func TestMultipartPutJoinsParts(t *testing.T) {
	f := &fakeMultipart{complete: "ok"}
	s := multipartStore(t, f)
	data := bigObject()
	id, err := s.Put(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	got := f.objects["p/"+id.String()]
	f.mu.Unlock()
	if !bytes.Equal(got, data) {
		t.Fatalf("joined object has %d bytes, want %d", len(got), len(data))
	}
	if atomic.LoadInt32(&f.aborts) != 0 {
		t.Fatal("a successful upload was aborted")
	}
}
