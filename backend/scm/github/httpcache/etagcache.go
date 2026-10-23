package httpcache

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"sync"
)

type etagCache struct {
	next http.RoundTripper

	mu      sync.Mutex
	entries map[string]cacheEntry
}

// cacheEntry is a stored 200 response, holding everything needed to rebuild it
// when the server later answers 304. Entries are never mutated after being put
// in the map, so copying one out by value is safe despite the shared slice and
// map.
type cacheEntry struct {
	etag   string
	header http.Header
	body   []byte
}

// NewETagCache returns an http.RoundTripper that turns repeated GETs into
// conditional requests. It remembers the ETag and body of every cacheable
// response, sends If-None-Match on the next GET for the same URL, and replays
// the stored body when the server answers 304 Not Modified. Requests are
// forwarded to next, or to http.DefaultTransport when next is nil. The
// returned tripper is safe for concurrent use.
//
// Conditional requests answered with a 304 do not count against GitHub's
// primary rate limit, which is what keeps the 3s build-status poll affordable:
// an unchanged status costs nothing, a real change costs one request.
//
// Freshness caching, serving a response without contacting GitHub at all for as
// long as its max-age allows, is deliberately absent. GitHub sends max-age=60
// on these endpoints, so honouring it would hide a newly triggered build for up
// to a minute.
//
// Each call returns an independent cache, so giving every client its own keeps
// responses from leaking between PATs. Entries are keyed by URL alone and are
// never evicted. Both are sound here because a client is short-lived and talks
// to a small, fixed set of endpoints with a constant Accept header.
func NewETagCache(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &etagCache{next: next, entries: make(map[string]cacheEntry)}
}

func (c *etagCache) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodGet {
		return c.roundTrip(req)
	}

	key := req.URL.String()
	entry, cached := c.lookup(key)
	if cached {
		// Clone before touching headers: a RoundTripper must not modify the
		// request it is given.
		req = req.Clone(req.Context())
		req.Header.Set("If-None-Match", entry.etag)
	}

	resp, err := c.roundTrip(req)
	if err != nil {
		return nil, err
	}

	if cached && resp.StatusCode == http.StatusNotModified {
		entry.replay(resp)
		return resp, nil
	}
	if err := c.store(key, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *etagCache) roundTrip(req *http.Request) (*http.Response, error) {
	resp, err := c.next.RoundTrip(req)
	if err != nil {
		return nil, fmt.Errorf("round trip: %w", err)
	}
	return resp, nil
}

func (c *etagCache) lookup(key string) (cacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	return entry, ok
}

// store buffers a cacheable response so a later 304 can replay it, leaving
// resp with its body intact. Anything not cacheable is left untouched and
// still streaming.
//
// Reading the body here, rather than waiting for the caller to drain it, is
// what makes caching deterministic: go-github decodes with a json.Decoder,
// which stops as soon as the top-level value is complete and so may never
// observe EOF on a chunked response.
func (c *etagCache) store(key string, resp *http.Response) error {
	etag := resp.Header.Get("ETag")
	if resp.StatusCode != http.StatusOK || etag == "" || resp.Body == nil {
		return nil
	}

	body, readErr := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	if readErr != nil {
		return fmt.Errorf("read response body for cache: %w", readErr)
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))

	// The body arrived whole, so the caller gets its response either way. A
	// failed Close only puts the connection in doubt, and the cost of not
	// storing the entry is one unconditional GET next time.
	if closeErr != nil {
		return nil
	}

	c.mu.Lock()
	c.entries[key] = cacheEntry{etag: etag, header: resp.Header.Clone(), body: body}
	c.mu.Unlock()

	return nil
}

// replay rebuilds the stored 200 response in place of the 304. The live
// response is reused so its Proto, TLS and Request fields survive; only the
// status, headers and body are swapped.
//
// Headers from the 304 overwrite the stored ones, so the caller sees current
// rate-limit counters and dates. Headers a 304 omits survive from the stored
// response, which matters for Link: it drives go-github's pagination, and
// losing it would silently truncate paginated results to the first page.
func (e cacheEntry) replay(notModified *http.Response) {
	if notModified.Body != nil {
		// A 304 carries no body, but the connection is only reusable once the
		// response is closed.
		_ = notModified.Body.Close()
	}

	header := e.header.Clone()
	for name, values := range notModified.Header {
		if hopByHopHeaders[http.CanonicalHeaderKey(name)] {
			continue
		}
		header[name] = values
	}

	notModified.StatusCode = http.StatusOK
	notModified.Status = fmt.Sprintf("%d %s", http.StatusOK, http.StatusText(http.StatusOK))
	notModified.Header = header
	notModified.Body = io.NopCloser(bytes.NewReader(e.body))
	notModified.ContentLength = int64(len(e.body))
}

// hopByHopHeaders are connection scoped (RFC 9110, section 7.6.1). They
// describe the 304 exchange rather than the stored representation, so they are
// not carried over onto the replayed response.
var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}
