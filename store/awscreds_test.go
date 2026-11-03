package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Avalanche-io/c4"
)

// Ported from c4-operator internal/storeurl/s3creds_test.go at 8654b0ce,
// adapted to the CLI's S3Store.

// A session token travels as X-Amz-Security-Token and is signed; a
// request re-signed without one drops the stale token.
func TestSessionTokenIsSentAndSigned(t *testing.T) {
	s := NewS3StoreWithCredentials("b", "k/", "us-east-1", "", Static(Credentials{AccessKeyID: "A", SecretAccessKey: "S", SessionToken: "T"}))
	req, _ := http.NewRequest(http.MethodGet, "https://b.s3.us-east-1.amazonaws.com/k", nil)
	if err := s.signRequest(req, "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("X-Amz-Security-Token") != "T" || !strings.Contains(req.Header.Get("Authorization"), "x-amz-security-token") {
		t.Fatalf("token not signed: %v", req.Header)
	}
	s.creds = Static(Credentials{AccessKeyID: "A", SecretAccessKey: "S"})
	if err := s.signRequest(req, "UNSIGNED-PAYLOAD"); err != nil {
		t.Fatal(err)
	}
	if req.Header.Get("X-Amz-Security-Token") != "" || strings.Contains(req.Header.Get("Authorization"), "x-amz-security-token") {
		t.Fatalf("a stale token was sent: %v", req.Header)
	}
}

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// The environment comes first, with its session token.
func TestAmbientFromTheEnvironment(t *testing.T) {
	c := &Chain{Getenv: env(map[string]string{"AWS_ACCESS_KEY_ID": "AK", "AWS_SECRET_ACCESS_KEY": "SK", "AWS_SESSION_TOKEN": "TOK"})}
	got, err := c.Retrieve(context.Background())
	if err != nil || got.AccessKeyID != "AK" || got.SessionToken != "TOK" || !got.Expires.IsZero() {
		t.Fatalf("%+v %v", got, err)
	}
	c = &Chain{Getenv: env(map[string]string{"AWS_ACCESS_KEY_ID": "AK"})}
	if _, err := c.Retrieve(context.Background()); err == nil {
		t.Fatal("a key ID with no secret was accepted")
	}
	// nothing in the environment, no web identity: the instance role is
	// never reached unless asked for
	c = &Chain{Getenv: env(nil), IMDSEndpoint: "http://127.0.0.1:1"}
	if _, err := c.Retrieve(context.Background()); err == nil || !strings.Contains(err.Error(), "only when asked for") {
		t.Fatalf("reached for the instance role by default: %v", err)
	}
}

// clock is a time the test moves.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

// fakeSTS answers AssumeRoleWithWebIdentity with keys numbered by call,
// valid for an hour of the test's clock.
type fakeSTS struct {
	clk    *clock
	calls  int32 // atomic
	tokens []string
	mu     sync.Mutex
	delay  time.Duration
	fail   int32 // atomic; 1 = fail
}

func (f *fakeSTS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	if r.Method != http.MethodPost || r.Form.Get("Action") != "AssumeRoleWithWebIdentity" || r.Form.Get("RoleArn") != "arn:aws:iam::123456789012:role/c4" || r.Form.Get("RoleSessionName") == "" {
		http.Error(w, "<ErrorResponse><Error><Code>InvalidParameter</Code><Message>bad request</Message></Error></ErrorResponse>", 400)
		return
	}
	if atomic.LoadInt32(&f.fail) != 0 {
		http.Error(w, "<ErrorResponse><Error><Code>ServiceUnavailable</Code><Message>down</Message></Error></ErrorResponse>", 503)
		return
	}
	time.Sleep(f.delay)
	f.mu.Lock()
	f.tokens = append(f.tokens, r.Form.Get("WebIdentityToken"))
	f.mu.Unlock()
	n := atomic.AddInt32(&f.calls, 1)
	exp := f.clk.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	fmt.Fprintf(w, `<AssumeRoleWithWebIdentityResponse><AssumeRoleWithWebIdentityResult><Credentials>
<AccessKeyId>ASIA%d</AccessKeyId><SecretAccessKey>secret%d</SecretAccessKey><SessionToken>token%d</SessionToken><Expiration>%s</Expiration>
</Credentials></AssumeRoleWithWebIdentityResult></AssumeRoleWithWebIdentityResponse>`, n, n, n, exp)
}

func webIdentityChain(t *testing.T, f *fakeSTS) (*Chain, string) {
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	tokFile := filepath.Join(t.TempDir(), "token")
	os.WriteFile(tokFile, []byte("jwt-1\n"), 0o600)
	return &Chain{
		Getenv:      env(map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": tokFile, "AWS_ROLE_ARN": "arn:aws:iam::123456789012:role/c4"}),
		STSEndpoint: srv.URL,
		Now:         f.clk.Now,
	}, tokFile
}

// A pod's web identity: exchanged with STS, kept until close to expiry,
// renewed with the token file as it is then (the kubelet rotates it), and
// kept through a failed renewal while it still holds.
func TestAmbientWebIdentityRenews(t *testing.T) {
	clk := &clock{t: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	f := &fakeSTS{clk: clk}
	c, tokFile := webIdentityChain(t, f)
	ctx := context.Background()

	got, err := c.Retrieve(ctx)
	if err != nil || got.AccessKeyID != "ASIA1" || got.SessionToken != "token1" {
		t.Fatalf("%+v %v", got, err)
	}
	clk.Add(50 * time.Minute) // 10 minutes left: still fresh
	if got, _ := c.Retrieve(ctx); got.AccessKeyID != "ASIA1" || atomic.LoadInt32(&f.calls) != 1 {
		t.Fatalf("renewed early: %s, %d calls", got.AccessKeyID, atomic.LoadInt32(&f.calls))
	}
	os.WriteFile(tokFile, []byte("jwt-2\n"), 0o600)
	clk.Add(6 * time.Minute) // 4 minutes left: renewal starts, the old keys serve meanwhile
	if got, _ := c.Retrieve(ctx); got.AccessKeyID != "ASIA1" {
		t.Fatalf("waited for the renewal: %s", got.AccessKeyID)
	}
	eventually(t, func() bool { got, _ := c.Retrieve(ctx); return got.AccessKeyID == "ASIA2" })
	if f.tokens[1] != "jwt-2" {
		t.Fatalf("renewed with the old token file: %q", f.tokens)
	}

	// STS goes down near expiry: the keys that still hold are used.
	atomic.StoreInt32(&f.fail, 1)
	clk.Add(57 * time.Minute) // 3 minutes left
	if got, err := c.Retrieve(ctx); err != nil || got.AccessKeyID != "ASIA2" {
		t.Fatalf("lost keys that still hold: %+v %v", got, err)
	}
	eventually(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.lastErr != nil })
	if got, err := c.Retrieve(ctx); err != nil || got.AccessKeyID != "ASIA2" {
		t.Fatalf("lost keys that still hold after a failed renewal: %+v %v", got, err)
	}
	clk.Add(4 * time.Minute) // expired, STS still down
	if _, err := c.Retrieve(ctx); err == nil || !strings.Contains(err.Error(), "ServiceUnavailable") {
		t.Fatalf("expired keys were used, or the error was lost: %v", err)
	}
	atomic.StoreInt32(&f.fail, 0)
	if _, err := c.Retrieve(ctx); err == nil {
		t.Fatal("asked STS again within 10 s of a failure")
	}
	clk.Add(11 * time.Second)
	if got, err := c.Retrieve(ctx); err != nil || got.AccessKeyID != "ASIA3" {
		t.Fatalf("no recovery: %+v %v", got, err)
	}
}

func eventually(t *testing.T, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatal("never happened")
}

// Many requests at once start one exchange: those holding no keys wait
// for it, and none start their own.
func TestAmbientRenewsOnceUnderLoad(t *testing.T) {
	clk := &clock{t: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	f := &fakeSTS{clk: clk, delay: 50 * time.Millisecond}
	c, _ := webIdentityChain(t, f)
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := c.Retrieve(context.Background())
			if err == nil && got.AccessKeyID != "ASIA1" {
				err = fmt.Errorf("got %s", got.AccessKeyID)
			}
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if n := atomic.LoadInt32(&f.calls); n != 1 {
		t.Fatalf("%d exchanges for 64 requests", n)
	}
}

// An EC2 instance's role, through IMDSv2: a session token first, then the
// role's keys, each asked with it. Only when asked for.
func TestAmbientInstanceRole(t *testing.T) {
	exp := time.Now().Add(6 * time.Hour).UTC().Format(time.RFC3339)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/latest/api/token" && r.Header.Get("X-Aws-Ec2-Metadata-Token-Ttl-Seconds") != "":
			io.WriteString(w, "imds-token")
		case r.Header.Get("X-Aws-Ec2-Metadata-Token") != "imds-token":
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Path == "/latest/meta-data/iam/security-credentials/":
			io.WriteString(w, "c4-cli\n")
		case r.URL.Path == "/latest/meta-data/iam/security-credentials/c4-cli":
			fmt.Fprintf(w, `{"Code":"Success","Type":"AWS-HMAC","AccessKeyId":"ASIAROLE","SecretAccessKey":"rs","Token":"rt","Expiration":%q}`, exp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := &Chain{InstanceRole: true, Getenv: env(nil), IMDSEndpoint: srv.URL}
	got, err := c.Retrieve(context.Background())
	if err != nil || got.AccessKeyID != "ASIAROLE" || got.SessionToken != "rt" || got.Expires.IsZero() {
		t.Fatalf("%+v %v", got, err)
	}
	c = &Chain{InstanceRole: true, Getenv: env(map[string]string{"AWS_EC2_METADATA_DISABLED": "true"}), IMDSEndpoint: srv.URL}
	if _, err := c.Retrieve(context.Background()); err == nil {
		t.Fatal("used the metadata service it was told not to")
	}
	c = &Chain{InstanceRole: false, Getenv: env(nil), IMDSEndpoint: srv.URL}
	if _, err := c.Retrieve(context.Background()); err == nil {
		t.Fatal("used the instance role without being asked")
	}
}

// fakeS3 is a bucket that accepts only the keys STS gave most recently,
// with their token, and holds single uploads.
type fakeS3 struct {
	mu      sync.Mutex
	valid   map[string]string // access key -> token
	objects map[string][]byte
}

var credRE = regexp.MustCompile(`Credential=([^/]+)/`)

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := credRE.FindStringSubmatch(r.Header.Get("Authorization"))
	if m == nil || f.valid[m[1]] == "" || r.Header.Get("X-Amz-Security-Token") != f.valid[m[1]] ||
		!strings.Contains(r.Header.Get("Authorization"), "x-amz-security-token") {
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, "<Error><Code>ExpiredToken</Code><Message>The provided token has expired.</Message></Error>")
		return
	}
	body, _ := io.ReadAll(r.Body)
	if r.Header.Get("X-Amz-Content-Sha256") != "UNSIGNED-PAYLOAD" {
		if sum := sha256.Sum256(body); r.Header.Get("X-Amz-Content-Sha256") != hex.EncodeToString(sum[:]) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
	}
	key := strings.TrimPrefix(r.URL.Path, "/bucket/")
	switch r.Method {
	case http.MethodHead:
		if b, ok := f.objects[key]; ok {
			w.Header().Set("Content-Length", fmt.Sprint(len(b)))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	case http.MethodPut:
		f.objects[key] = body
	case http.MethodGet:
		if b, ok := f.objects[key]; ok {
			w.Write(b)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// A store on a pod's web identity keeps working as its keys expire mid-run:
// the bucket refuses the old keys once STS has issued new ones, and every
// request after the renewal is signed with the new keys and token.
func TestS3StoreSurvivesExpiryMidRun(t *testing.T) {
	clk := &clock{t: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	bucket := &fakeS3{valid: map[string]string{}, objects: map[string][]byte{}}
	sts := &fakeSTS{clk: clk}
	stsSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := httptest.NewRecorder()
		sts.ServeHTTP(rec, r)
		// What STS issues, the bucket accepts; it forgets older keys.
		n := atomic.LoadInt32(&sts.calls)
		bucket.mu.Lock()
		bucket.valid = map[string]string{fmt.Sprintf("ASIA%d", n): fmt.Sprintf("token%d", n)}
		bucket.mu.Unlock()
		for k, v := range rec.Header() {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.Code)
		w.Write(rec.Body.Bytes())
	}))
	defer stsSrv.Close()
	s3Srv := httptest.NewServer(bucket)
	defer s3Srv.Close()
	tokFile := filepath.Join(t.TempDir(), "token")
	os.WriteFile(tokFile, []byte("jwt"), 0o600)
	chain := &Chain{
		Getenv:      env(map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": tokFile, "AWS_ROLE_ARN": "arn:aws:iam::123456789012:role/c4"}),
		STSEndpoint: stsSrv.URL,
		Now:         clk.Now,
	}
	s := NewS3StoreWithCredentials("bucket", "runs/", "us-east-1", s3Srv.URL, chain)
	for i := 0; i < 4; i++ {
		data := []byte(fmt.Sprintf("object %d", i))
		id, err := s.Put(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
		if !s.Has(id) {
			t.Fatalf("has %d: false", i)
		}
		if _, ok := bucket.objects["runs/"+id.String()]; !ok {
			t.Fatalf("object %d not under the prefix", i)
		}
		rc, err := s.Open(id)
		if err != nil {
			t.Fatalf("open %d: %v", i, err)
		}
		got, _ := io.ReadAll(rc)
		rc.Close()
		if !bytes.Equal(got, data) {
			t.Fatalf("open %d: %q", i, got)
		}
		clk.Add(40 * time.Minute) // across two renewals in four objects
	}
	if n := atomic.LoadInt32(&sts.calls); n < 2 {
		t.Fatalf("keys never renewed (%d exchanges)", n)
	}
}

// The prefix must end in "/" (or be empty), as the operator requires, so
// both name the same keys; the error names the fix.
func TestS3PrefixMustEndInSlash(t *testing.T) {
	setenv(t, "AWS_ACCESS_KEY_ID", "AK")
	setenv(t, "AWS_SECRET_ACCESS_KEY", "SK")
	for raw, ok := range map[string]bool{
		"s3://bucket":            true,
		"s3://bucket/":           true,
		"s3://bucket/runs/":      true,
		"s3://bucket/a/b/":       true,
		"s3://bucket/runs":       false,
		"s3://bucket/c4-bench-x": false,
	} {
		_, err := openS3Configured(raw)
		if (err == nil) != ok {
			t.Errorf("%s: %v", raw, err)
		}
		if err != nil && !strings.Contains(err.Error(), "must end in \"/\"") {
			t.Errorf("%s: the error should name the fix: %v", raw, err)
		}
	}
	// a store without keys anywhere opens (the chain resolves per
	// request), and instance-role must be a boolean
	setenv(t, "AWS_ACCESS_KEY_ID", "")
	setenv(t, "AWS_SECRET_ACCESS_KEY", "")
	if _, err := openS3Configured("s3://bucket/runs/?instance-role=maybe"); err == nil {
		t.Error("instance-role=maybe was accepted")
	}
	s, err := openS3Configured("s3://bucket/runs/?instance-role=true")
	if err != nil || s == nil {
		t.Fatalf("open with the chain: %v", err)
	}
	if ch, ok := s.creds.(*Chain); !ok || !ch.InstanceRole {
		t.Errorf("instance-role=true should opt the chain into the instance role")
	}
	s, err = openS3Configured("s3://bucket/runs/")
	if err != nil {
		t.Fatal(err)
	}
	if ch, ok := s.creds.(*Chain); !ok || ch.InstanceRole {
		t.Errorf("the instance role must be off unless asked for")
	}
	_ = c4.ID{}
}

// setenv sets an environment variable for the test and restores it after
// (testing.T.Setenv needs Go 1.17; the matrix builds on 1.16).
func setenv(t *testing.T, k, v string) {
	t.Helper()
	old, had := os.LookupEnv(k)
	os.Setenv(k, v)
	t.Cleanup(func() {
		if had {
			os.Setenv(k, old)
		} else {
			os.Unsetenv(k)
		}
	})
}
