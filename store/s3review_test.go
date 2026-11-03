package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The security review's tests, ported from c4-operator's
// internal/storeurl/s3review_test.go (branch s3-session-creds, e9ff1c26)
// so they live with the chain they protect.

const reviewToken = "FQoGZXIvYXdzE-review-token"

// No client follows a redirect: not S3 (the token and the body would go
// to the Location's host, and a 302 would turn a PUT into a body-less
// GET that "succeeds"), not STS (the web identity token would be re-posted,
// even to plain http), not the metadata service (its token would go too).
func TestNoRedirectIsFollowed(t *testing.T) {
	var saw atomic.Value
	saw.Store("")
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		saw.Store(r.Method + " " + r.Header.Get("X-Amz-Security-Token") + r.Form.Get("WebIdentityToken") + r.Header.Get("X-Aws-Ec2-Metadata-Token"))
		io.ReadAll(r.Body)
		io.WriteString(w, "ok")
	}))
	defer evil.Close()
	elsewhere := strings.Replace(evil.URL, "127.0.0.1", "localhost", 1)

	for _, code := range []int{http.StatusTemporaryRedirect, http.StatusFound} {
		s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, elsewhere+r.URL.Path, code)
		}))
		s := NewS3StoreWithCredentials("bucket", "p/", "us-east-1", s3.URL,
			Static(Credentials{AccessKeyID: "AK", SecretAccessKey: "SK", SessionToken: reviewToken}))
		data := []byte("important bytes")
		_, err := s.Put(bytes.NewReader(data))
		s3.Close()
		if err == nil {
			t.Errorf("S3 %d: a redirected PUT counted as stored", code)
		}
		if saw.Load() != "" {
			t.Errorf("S3 %d: followed to %q", code, saw.Load())
		}
	}

	sts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/steal", http.StatusTemporaryRedirect)
	}))
	defer sts.Close()
	tok := filepath.Join(t.TempDir(), "token")
	os.WriteFile(tok, []byte("eyJ-probe-jwt"), 0o600)
	c := &Chain{Getenv: env(map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": tok, "AWS_ROLE_ARN": "arn:aws:iam::1:role/r"}), STSEndpoint: sts.URL, Client: sts.Client()}
	if _, err := c.Retrieve(context.Background()); err == nil {
		t.Error("STS: a redirect gave credentials")
	}
	if saw.Load() != "" {
		t.Errorf("STS: followed to %q", saw.Load())
	}

	imds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			io.WriteString(w, "imds-session")
			return
		}
		http.Redirect(w, r, elsewhere+r.URL.Path, http.StatusFound)
	}))
	defer imds.Close()
	c = &Chain{InstanceRole: true, Getenv: env(nil), IMDSEndpoint: imds.URL}
	if _, err := c.Retrieve(context.Background()); err == nil {
		t.Error("IMDS: a redirect gave credentials")
	}
	if saw.Load() != "" {
		t.Errorf("IMDS: followed to %q", saw.Load())
	}
}

// Keys that arrive already expired (a skewed clock, a bad endpoint) are an
// error, not keys; and the callers that meet the failure don't each start
// their own exchange.
func TestExpiredOnArrivalIsAnError(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond)
		exp := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
		fmt.Fprintf(w, `<R><AssumeRoleWithWebIdentityResult><Credentials><AccessKeyId>ASIA%d</AccessKeyId><SecretAccessKey>s</SecretAccessKey><SessionToken>t</SessionToken><Expiration>%s</Expiration></Credentials></AssumeRoleWithWebIdentityResult></R>`, n, exp)
	}))
	defer srv.Close()
	tok := filepath.Join(t.TempDir(), "token")
	os.WriteFile(tok, []byte("jwt"), 0o600)
	c := &Chain{Getenv: env(map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": tok, "AWS_ROLE_ARN": "arn:aws:iam::1:role/r"}), STSEndpoint: srv.URL}
	if got, err := c.Retrieve(context.Background()); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired keys: %v, %v", got, err)
	}
	atomic.StoreInt32(&calls, 0)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Retrieve(context.Background()); err == nil {
				t.Error("expired keys handed out")
			}
		}()
	}
	wg.Wait()
	if n := atomic.LoadInt32(&calls); n > 1 {
		t.Fatalf("%d exchanges for 16 callers after a failure", n)
	}
}

// A caller whose keys still hold never waits for the renewal it starts,
// and a caller with none waits only as long as its context.
func TestRenewalNeverHoldsACaller(t *testing.T) {
	clk := &clock{t: time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)}
	f := &fakeSTS{clk: clk}
	c, _ := webIdentityChain(t, f)
	if _, err := c.Retrieve(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.delay = 1500 * time.Millisecond
	clk.Add(56 * time.Minute) // 4 minutes left: due for renewal
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	got, err := c.Retrieve(ctx)
	if el := time.Since(start); el > 50*time.Millisecond || err != nil || got.AccessKeyID != "ASIA1" {
		t.Fatalf("with keys that hold: %v, %s, %v", el, got.AccessKeyID, err)
	}
	clk.Add(5 * time.Minute) // expired, renewal still running
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel2()
	start = time.Now()
	if _, err := c.Retrieve(ctx2); !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("with none: %v after %v", err, time.Since(start))
	}
}

// However Credentials is formatted, its secret and token don't show
// (the slog form is checked in s3review_slog_test.go).
func TestCredentialsNeverPrintTheirSecrets(t *testing.T) {
	c := Credentials{AccessKeyID: "AK", SecretAccessKey: "SECRETVAL", SessionToken: "TOKVAL", Expires: time.Unix(1800000000, 0)}
	for _, s := range []string{fmt.Sprint(c), fmt.Sprintf("%v %+v %#v %s", c, c, c, c), fmt.Sprintf("%v", &c)} {
		if strings.Contains(s, "SECRETVAL") || strings.Contains(s, "TOKVAL") || !strings.Contains(s, "AK") {
			t.Errorf("printed as %s", s)
		}
	}
}

// STS keys with no expiration are refused, without quoting them.
func TestMissingExpirationIsRefused(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `<R><AssumeRoleWithWebIdentityResult><Credentials><AccessKeyId>ASIAX</AccessKeyId><SecretAccessKey>SECRETVAL</SecretAccessKey><SessionToken>TOKVAL</SessionToken></Credentials></AssumeRoleWithWebIdentityResult></R>`)
	}))
	defer srv.Close()
	tok := filepath.Join(t.TempDir(), "token")
	os.WriteFile(tok, []byte("jwt"), 0o600)
	c := &Chain{Getenv: env(map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": tok, "AWS_ROLE_ARN": "arn:aws:iam::1:role/r"}), STSEndpoint: srv.URL}
	_, err := c.Retrieve(context.Background())
	if err == nil || strings.Contains(err.Error(), "SECRETVAL") || strings.Contains(err.Error(), "TOKVAL") {
		t.Fatalf("missing expiration: %v", err)
	}
}

// A region becomes part of a host name, so it must be a region name.
func TestRegionMustBeARegionName(t *testing.T) {
	setenv(t, "AWS_ACCESS_KEY_ID", "AK")
	setenv(t, "AWS_SECRET_ACCESS_KEY", "SK")
	for _, r := range []string{"evil.example/x", "us-east-1@evil", "US-EAST-1", "a b"} {
		if _, err := openS3Configured("s3://bucket/?region=" + strings.ReplaceAll(r, " ", "%20")); err == nil {
			t.Errorf("region %q accepted", r)
		}
	}
	if _, err := openS3Configured("s3://bucket/?region=eu-west-3"); err != nil {
		t.Error(err)
	}
	c := &Chain{Getenv: env(map[string]string{"AWS_WEB_IDENTITY_TOKEN_FILE": "/x", "AWS_ROLE_ARN": "r", "AWS_REGION": "evil.example/"})}
	if _, err := c.stsEndpoint(); err == nil {
		t.Error("AWS_REGION not checked")
	}
}

// Without instance-role=true the chain never asks the metadata service:
// a process whose keys are missing fails rather than writing as the
// machine.
func TestInstanceRoleOnlyWhenAsked(t *testing.T) {
	var asked int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { atomic.AddInt32(&asked, 1) }))
	defer srv.Close()
	c := &Chain{Getenv: env(nil), IMDSEndpoint: srv.URL}
	if _, err := c.Retrieve(context.Background()); err == nil || atomic.LoadInt32(&asked) != 0 {
		t.Fatalf("without the opt-in: %v, metadata asked %d times", err, atomic.LoadInt32(&asked))
	}
}
