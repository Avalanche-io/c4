package store

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// signingKeyCache caches the derived signing key for a given date so that
// the four-step HMAC chain is computed at most once per calendar day.
type signingKeyCache struct {
	mu   sync.Mutex
	date string
	key  []byte
}

// signRequest adds AWS Signature V4 authorization headers to req.
// payloadHash must be the hex-encoded SHA-256 of the request body,
// or "UNSIGNED-PAYLOAD" for requests with no meaningful body.
func (s *S3Store) signRequest(req *http.Request, payloadHash string) {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")
	amzDate := now.Format("20060102T150405Z")

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", payloadHash)

	// Ensure Host header is set (some HTTP clients omit it from Header map).
	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", req.Host)
	}

	// --- Canonical request ---
	signedHeaders, canonicalHeaders := canonicalHeaderString(req)
	canonicalURI := uriEncodePath(req.URL.Path)
	canonicalQuery := canonicalQueryString(req)

	canonical := strings.Join([]string{
		req.Method,
		canonicalURI,
		canonicalQuery,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	// --- String to sign ---
	scope := dateStamp + "/" + s.region + "/s3/aws4_request"
	canonicalHash := hashSHA256([]byte(canonical))
	stringToSign := "AWS4-HMAC-SHA256\n" + amzDate + "\n" + scope + "\n" + canonicalHash

	// --- Signing key ---
	signingKey := s.deriveKey(dateStamp)

	// --- Signature ---
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	// --- Authorization header ---
	auth := "AWS4-HMAC-SHA256 Credential=" + s.accessKey + "/" + scope +
		", SignedHeaders=" + signedHeaders +
		", Signature=" + signature
	req.Header.Set("Authorization", auth)
}

// deriveKey returns the signing key for the given date, using a cache to
// avoid recomputation on every request.
func (s *S3Store) deriveKey(dateStamp string) []byte {
	s.keyCache.mu.Lock()
	defer s.keyCache.mu.Unlock()

	if s.keyCache.date == dateStamp && s.keyCache.key != nil {
		return s.keyCache.key
	}

	kDate := hmacSHA256([]byte("AWS4"+s.secretKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(s.region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))

	s.keyCache.date = dateStamp
	s.keyCache.key = kSigning
	return kSigning
}

// canonicalHeaderString builds the sorted, lowercased canonical headers
// block and the signed-headers list. Returns (signedHeaders, canonicalHeaders).
func canonicalHeaderString(req *http.Request) (string, string) {
	// Collect header names we sign: host, content-type, and all x-amz-*.
	type hdr struct {
		key string
		val string
	}
	var headers []hdr
	for k := range req.Header {
		lower := strings.ToLower(k)
		if lower == "host" || lower == "content-type" || strings.HasPrefix(lower, "x-amz-") {
			// Trim and collapse whitespace per spec.
			val := strings.TrimSpace(req.Header.Get(k))
			headers = append(headers, hdr{lower, val})
		}
	}
	sort.Slice(headers, func(i, j int) bool {
		return headers[i].key < headers[j].key
	})

	var names []string
	var lines []string
	for _, h := range headers {
		names = append(names, h.key)
		lines = append(lines, h.key+":"+h.val)
	}

	// Canonical headers block ends with a trailing newline.
	canonicalHeaders := strings.Join(lines, "\n") + "\n"
	signedHeaders := strings.Join(names, ";")
	return signedHeaders, canonicalHeaders
}

// canonicalQueryString returns the query parameters sorted by key name,
// each key and value URI-encoded.
func canonicalQueryString(req *http.Request) string {
	query := req.URL.Query()
	if len(query) == 0 {
		return ""
	}
	var pairs []string
	for k, vals := range query {
		for _, v := range vals {
			pairs = append(pairs, uriEncode(k, true)+"="+uriEncode(v, true))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

// uriEncodePath encodes a URI path per AWS SigV4 rules: every component
// is percent-encoded, but forward slashes are preserved.
func uriEncodePath(path string) string {
	if path == "" {
		return "/"
	}
	segments := strings.Split(path, "/")
	for i, seg := range segments {
		segments[i] = uriEncode(seg, false)
	}
	return strings.Join(segments, "/")
}

// uriEncode percent-encodes s. If encodeSlash is true, '/' is also encoded.
// Unreserved characters A-Z a-z 0-9 - _ . ~ are never encoded.
func uriEncode(s string, encodeSlash bool) string {
	var buf strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isUnreserved(c) || (c == '/' && !encodeSlash) {
			buf.WriteByte(c)
		} else {
			buf.WriteByte('%')
			buf.WriteByte(hexUpper[c>>4])
			buf.WriteByte(hexUpper[c&0x0f])
		}
	}
	return buf.String()
}

func isUnreserved(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' || c == '~'
}

const hexUpper = "0123456789ABCDEF"

// hashSHA256 returns the lowercase hex-encoded SHA-256 of data.
func hashSHA256(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// hmacSHA256 computes HMAC-SHA256.
func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}
