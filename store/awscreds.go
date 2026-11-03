package store

import (
	"bufio"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Credentials carries S3 keys. Temporary credentials (a web identity, an
// instance role) also carry a session token and the time they expire.
// Ported from c4-operator internal/storeurl/creds.go at 8654b0ce.
type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expires         time.Time // zero: they don't expire
}

func (c Credentials) ok() bool { return c.AccessKeyID != "" && c.SecretAccessKey != "" }

func (c Credentials) validAt(t time.Time) bool {
	return c.ok() && (c.Expires.IsZero() || t.Before(c.Expires))
}

// String and GoString (and LogValue, in awscreds_slog.go for Go 1.21
// and later) name the key and never print the secret or the token,
// however Credentials is formatted or logged.
func (c Credentials) String() string {
	s := "Credentials{AccessKeyID: " + c.AccessKeyID + ", SecretAccessKey: " + redacted(c.SecretAccessKey)
	if c.SessionToken != "" {
		s += ", SessionToken: " + redacted(c.SessionToken)
	}
	if !c.Expires.IsZero() {
		s += ", Expires: " + c.Expires.UTC().Format(time.RFC3339)
	}
	return s + "}"
}

func (c Credentials) GoString() string { return c.String() }

func redacted(v string) string {
	if v == "" {
		return `""`
	}
	return "<redacted>"
}

// CredentialSource gives the credentials to sign the next request with.
type CredentialSource interface {
	Retrieve(ctx context.Context) (Credentials, error)
}

type staticSource Credentials

func (s staticSource) Retrieve(context.Context) (Credentials, error) { return Credentials(s), nil }

// Static is a source that always gives c: keys from a Secret, or from a
// person's environment.
func Static(c Credentials) CredentialSource { return staticSource(c) }

// Ambient is the standard AWS credential chain, as the AWS CLI and SDKs
// read it, for a process running as itself (a CLI, a job):
//
//  1. the environment: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY and, for
//     temporary keys, AWS_SESSION_TOKEN;
//  2. a web identity (EKS pod roles, IRSA): AWS_WEB_IDENTITY_TOKEN_FILE and
//     AWS_ROLE_ARN, exchanged with STS for temporary keys;
//  3. only with instanceRole, the machine's role, from the EC2 metadata
//     service (IMDSv2), unless AWS_EC2_METADATA_DISABLED=true.
//
// The machine's role is never reached by default: a process whose keys
// are missing must fail, not write quietly as the machine. The CLI opts
// in with instance-role=true on the store URL. Temporary keys are renewed
// before they expire. This is the one credential chain of the C4 suite;
// c4-operator imports it from here.
func Ambient(instanceRole bool) CredentialSource { return &Chain{InstanceRole: instanceRole} }

// Chain is Ambient's implementation; its fields are for tests.
type Chain struct {
	InstanceRole bool                // try the EC2 instance role last
	Getenv       func(string) string // default os.Getenv
	Client       *http.Client        // for STS; default a 30 s timeout. Redirects are never followed
	IMDSClient   *http.Client        // default a 2 s timeout: off EC2 it fails fast. Redirects are never followed
	STSEndpoint  string              // default from AWS_ENDPOINT_URL_STS or the region
	IMDSEndpoint string              // default AWS_EC2_METADATA_SERVICE_ENDPOINT or 169.254.169.254
	Now          func() time.Time

	mu        sync.Mutex
	cur       Credentials
	inflight  chan struct{}
	lastErr   error     // the last renewal's failure, until one succeeds
	lastFail  time.Time // when it failed
	sessionNo int
}

// Renew this long before temporary keys expire. STS and the instance
// role hand out keys valid for an hour or more, and renew them in their
// last 5 to 15 minutes.
const renewBefore = 5 * time.Minute

// After a failed renewal, nothing asks again for this long: callers with
// keys that still hold use them, and callers with none get the failure,
// so an outage costs one exchange every 10 s, not one per request.
const retryRenewal = 10 * time.Second

// A renewal gives up after this long.
const renewTimeout = time.Minute

func (c *Chain) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Chain) getenv(k string) string {
	if c.Getenv != nil {
		return c.Getenv(k)
	}
	return os.Getenv(k)
}

// Retrieve gives the current keys, starting a renewal when they are close
// to expiry. One renewal runs at a time, on its own: a caller holding keys
// that still hold never waits for it, and a caller holding none waits only
// as long as its own context allows.
func (c *Chain) Retrieve(ctx context.Context) (Credentials, error) {
	for {
		c.mu.Lock()
		now := c.now()
		valid := c.cur.validAt(now)
		if valid && (c.cur.Expires.IsZero() || now.Before(c.cur.Expires.Add(-renewBefore))) {
			cur := c.cur
			c.mu.Unlock()
			return cur, nil
		}
		backingOff := c.lastErr != nil && now.Sub(c.lastFail) < retryRenewal
		if c.inflight == nil && !backingOff {
			c.inflight = make(chan struct{})
			go c.renew(c.inflight)
		}
		switch {
		case valid:
			cur := c.cur
			c.mu.Unlock()
			return cur, nil
		case c.inflight == nil:
			err := c.lastErr
			c.mu.Unlock()
			return Credentials{}, err
		}
		ch := c.inflight
		c.mu.Unlock()
		select {
		case <-ch:
			c.mu.Lock()
			if !c.cur.validAt(c.now()) && c.lastErr != nil {
				err := c.lastErr
				c.mu.Unlock()
				return Credentials{}, err
			}
			c.mu.Unlock()
		case <-ctx.Done():
			return Credentials{}, ctx.Err()
		}
	}
}

// renew runs one renewal, detached from any caller's context.
func (c *Chain) renew(done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), renewTimeout)
	got, err := c.resolve(ctx)
	cancel()
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if err == nil && !got.validAt(now) {
		err = fmt.Errorf("renewed keys %s expired at %s, before they arrived (clock skew?)", got.AccessKeyID, got.Expires.UTC().Format(time.RFC3339))
	}
	if err == nil {
		c.cur, c.lastErr = got, nil
	} else {
		c.lastErr, c.lastFail = err, now
	}
	c.inflight = nil
	close(done)
}

func (c *Chain) resolve(ctx context.Context) (Credentials, error) {
	id, secret := c.getenv("AWS_ACCESS_KEY_ID"), c.getenv("AWS_SECRET_ACCESS_KEY")
	switch {
	case id != "" && secret != "":
		return Credentials{AccessKeyID: id, SecretAccessKey: secret, SessionToken: c.getenv("AWS_SESSION_TOKEN")}, nil
	case id != "" || secret != "":
		return Credentials{}, errors.New("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set together")
	}
	if file, role := c.getenv("AWS_WEB_IDENTITY_TOKEN_FILE"), c.getenv("AWS_ROLE_ARN"); file != "" || role != "" {
		if file == "" || role == "" {
			return Credentials{}, errors.New("a web identity needs both AWS_WEB_IDENTITY_TOKEN_FILE and AWS_ROLE_ARN")
		}
		return c.webIdentity(ctx, file, role)
	}
	if !c.InstanceRole {
		return Credentials{}, errors.New("no AWS credentials: none in the environment and no web identity (the machine's instance role is used only when asked for)")
	}
	if strings.EqualFold(c.getenv("AWS_EC2_METADATA_DISABLED"), "true") {
		return Credentials{}, errors.New("no AWS credentials: none in the environment, no web identity, and the instance metadata service is disabled")
	}
	creds, err := c.instanceRole(ctx)
	if err != nil {
		return Credentials{}, fmt.Errorf("no AWS credentials: none in the environment, no web identity, and no instance role (%w)", err)
	}
	return creds, nil
}

// ---- web identity (STS AssumeRoleWithWebIdentity) ----

func (c *Chain) stsEndpoint() (string, error) {
	if c.STSEndpoint != "" {
		return c.STSEndpoint, nil
	}
	if ep := c.getenv("AWS_ENDPOINT_URL_STS"); ep != "" {
		return ep, nil
	}
	region := c.getenv("AWS_REGION")
	if region == "" {
		region = c.getenv("AWS_DEFAULT_REGION")
	}
	switch {
	case region == "":
		return "https://sts.amazonaws.com", nil
	case !validRegion(region):
		return "", fmt.Errorf("AWS region %q is not a region name", region)
	case strings.HasPrefix(region, "cn-"):
		return "https://sts." + region + ".amazonaws.com.cn", nil
	}
	return "https://sts." + region + ".amazonaws.com", nil
}

// validRegion keeps a region name to what can safely become part of a
// host name.
func validRegion(r string) bool {
	if r == "" {
		return false
	}
	for _, ch := range r {
		if !(ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' || ch == '-') {
			return false
		}
	}
	return true
}

// noRedirects is cl (or a client with timeout) that never follows a
// redirect: a 3xx is the answer. Following one would resend the web
// identity token, a session token or a request body to wherever the
// Location points, and would turn a PUT into a body-less GET.
func noRedirects(cl *http.Client, timeout time.Duration) *http.Client {
	var c http.Client
	if cl != nil {
		c = *cl
	} else {
		c.Timeout = timeout
	}
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &c
}

func (c *Chain) webIdentity(ctx context.Context, file, role string) (Credentials, error) {
	// The token file is rotated by the kubelet; read it at every renewal.
	tok, err := os.ReadFile(file)
	if err != nil {
		return Credentials{}, fmt.Errorf("web identity token: %w", err)
	}
	session := c.getenv("AWS_ROLE_SESSION_NAME")
	if session == "" {
		c.mu.Lock()
		c.sessionNo++
		session = fmt.Sprintf("c4-%d-%d", c.now().Unix(), c.sessionNo)
		c.mu.Unlock()
	}
	form := url.Values{
		"Action":           {"AssumeRoleWithWebIdentity"},
		"Version":          {"2011-06-15"},
		"RoleArn":          {role},
		"RoleSessionName":  {session},
		"WebIdentityToken": {strings.TrimSpace(string(tok))},
	}
	endpoint, err := c.stsEndpoint()
	if err != nil {
		return Credentials{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Credentials{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := noRedirects(c.Client, 30*time.Second).Do(req)
	if err != nil {
		return Credentials{}, fmt.Errorf("sts: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Code    string `xml:"Error>Code"`
			Message string `xml:"Error>Message"`
		}
		xml.Unmarshal(body, &e)
		return Credentials{}, fmt.Errorf("sts AssumeRoleWithWebIdentity: status %d: %s %s", resp.StatusCode, e.Code, e.Message)
	}
	var out struct {
		Creds struct {
			AccessKeyID     string `xml:"AccessKeyId"`
			SecretAccessKey string `xml:"SecretAccessKey"`
			SessionToken    string `xml:"SessionToken"`
			Expiration      string `xml:"Expiration"`
		} `xml:"AssumeRoleWithWebIdentityResult>Credentials"`
	}
	if err := xml.Unmarshal(body, &out); err != nil {
		return Credentials{}, fmt.Errorf("sts: %w", err)
	}
	return temporary(out.Creds.AccessKeyID, out.Creds.SecretAccessKey, out.Creds.SessionToken, out.Creds.Expiration, "sts")
}

// ---- instance role (IMDSv2) ----

func (c *Chain) instanceRole(ctx context.Context) (Credentials, error) {
	base := c.IMDSEndpoint
	if base == "" {
		base = c.getenv("AWS_EC2_METADATA_SERVICE_ENDPOINT")
	}
	if base == "" {
		base = "http://169.254.169.254"
	}
	base = strings.TrimSuffix(base, "/")
	client := noRedirects(c.IMDSClient, 2*time.Second)
	call := func(method, path, token string) (string, error) {
		req, err := http.NewRequestWithContext(ctx, method, base+path, nil)
		if err != nil {
			return "", err
		}
		if token == "" {
			req.Header.Set("X-Aws-Ec2-Metadata-Token-Ttl-Seconds", "21600")
		} else {
			req.Header.Set("X-Aws-Ec2-Metadata-Token", token)
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("instance metadata %s: status %d", path, resp.StatusCode)
		}
		return string(body), nil
	}
	token, err := call(http.MethodPut, "/latest/api/token", "")
	if err != nil {
		return Credentials{}, err
	}
	roles, err := call(http.MethodGet, "/latest/meta-data/iam/security-credentials/", token)
	if err != nil {
		return Credentials{}, err
	}
	sc := bufio.NewScanner(strings.NewReader(roles))
	if !sc.Scan() || strings.TrimSpace(sc.Text()) == "" {
		return Credentials{}, errors.New("the instance has no role")
	}
	doc, err := call(http.MethodGet, "/latest/meta-data/iam/security-credentials/"+strings.TrimSpace(sc.Text()), token)
	if err != nil {
		return Credentials{}, err
	}
	var out struct {
		Code            string
		AccessKeyID     string `json:"AccessKeyId"`
		SecretAccessKey string
		Token           string
		Expiration      string
	}
	if err := json.Unmarshal([]byte(doc), &out); err != nil {
		return Credentials{}, fmt.Errorf("instance role: %w", err)
	}
	if out.Code != "" && out.Code != "Success" {
		return Credentials{}, fmt.Errorf("instance role: %s", out.Code)
	}
	return temporary(out.AccessKeyID, out.SecretAccessKey, out.Token, out.Expiration, "instance role")
}

func temporary(id, secret, token, expiration, from string) (Credentials, error) {
	exp, err := time.Parse(time.RFC3339, strings.TrimSpace(expiration))
	if err != nil {
		return Credentials{}, fmt.Errorf("%s: expiration %q: %w", from, expiration, err)
	}
	c := Credentials{AccessKeyID: id, SecretAccessKey: secret, SessionToken: token, Expires: exp}
	if !c.ok() || token == "" {
		return Credentials{}, fmt.Errorf("%s: incomplete credentials", from)
	}
	return c, nil
}
