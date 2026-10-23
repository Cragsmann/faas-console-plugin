package httpcache_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift/faas-console-plugin/backend/scm/github/httpcache"
)

var _ = Describe("etagCache", func() {

	It("replays the stored body when the server answers 304", func() {
		url := serve(mustCache())
		rt := httpcache.NewETagCache(http.DefaultTransport)

		_, first := doGetRequest(rt, url)
		resp, second := doGetRequest(rt, url)

		Expect(first).To(Equal("contents of /"))
		Expect(second).To(Equal(first), "anything else means the second GET went out without If-None-Match")
		Expect(resp.StatusCode).To(Equal(http.StatusOK), "a 304 reaches the caller as the cached 200")
		Expect(resp.ContentLength).To(Equal(int64(len(first))))
	})

	It("revalidates every time instead of honouring max-age", func() {
		// A representation that changes on every request. GitHub sends
		// max-age=60 on these endpoints, and a cache that honoured it would
		// hand back version 1 three times, hiding a newly triggered build for
		// up to a minute.
		var version atomic.Int32
		url := serve(func(w http.ResponseWriter, _ *http.Request) {
			n := version.Add(1)
			w.Header().Set("ETag", fmt.Sprintf(`"v%d"`, n))
			w.Header().Set("Cache-Control", "max-age=60")
			_, _ = fmt.Fprintf(w, "version %d", n)
		})
		rt := httpcache.NewETagCache(http.DefaultTransport)

		_, first := doGetRequest(rt, url)
		_, second := doGetRequest(rt, url)
		_, third := doGetRequest(rt, url)

		Expect([]string{first, second, third}).To(Equal([]string{"version 1", "version 2", "version 3"}))
	})

	It("serves a changed representation rather than the stale one", func() {
		etag, payload := `"v1"`, "first"
		url := serve(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("ETag", etag)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			_, _ = w.Write([]byte(payload))
		})
		rt := httpcache.NewETagCache(http.DefaultTransport)

		doGetRequest(rt, url)
		etag, payload = `"v2"`, "second"
		_, changed := doGetRequest(rt, url)
		_, afterwards := doGetRequest(rt, url)

		Expect(changed).To(Equal("second"))
		Expect(afterwards).To(Equal("second"), "the stale body must not come back once the cache has seen the new one")
	})

	It("keeps headers the 304 omits and refreshes the ones it sends", func() {
		url := serve(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("ETag", `"v1"`)
			w.Header().Set("X-RateLimit-Remaining", "4999")
			if r.Header.Get("If-None-Match") == `"v1"` {
				w.Header().Set("X-RateLimit-Remaining", "4998")
				w.WriteHeader(http.StatusNotModified)
				return
			}
			// Link drives go-github's pagination and is absent from a 304.
			w.Header().Set("Link", `<https://example.com/next>; rel="next"`)
			_, _ = w.Write([]byte("payload"))
		})
		rt := httpcache.NewETagCache(http.DefaultTransport)

		doGetRequest(rt, url)
		resp, _ := doGetRequest(rt, url)

		Expect(resp.Header.Get("Link")).To(Equal(`<https://example.com/next>; rel="next"`))
		Expect(resp.Header.Get("X-RateLimit-Remaining")).To(Equal("4998"))
	})

	It("caches bodies of any size, whatever the framing", func() {
		// go-github decodes with a json.Decoder, which can stop short of EOF.
		// Caching must not depend on the caller draining the body, nor on
		// whether the response is chunked, so this reads a single byte of a
		// body far larger than any buffer boundary and then closes it, the way
		// a decoder that is done might.
		url := serve(mustCachePadded(100_000))
		rt := httpcache.NewETagCache(http.DefaultTransport)

		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
		Expect(err).NotTo(HaveOccurred())
		resp, err := rt.RoundTrip(req)
		Expect(err).NotTo(HaveOccurred())
		buf := make([]byte, 1)
		_, err = resp.Body.Read(buf)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Body.Close()).To(Succeed())

		_, body := doGetRequest(rt, url)

		Expect(body).To(HavePrefix("contents of /"), "anything else means the partial read was never cached")
		Expect(body).To(HaveLen(len("contents of /") + 100_000))
	})

	It("does not cache a response without an ETag", func() {
		var version atomic.Int32
		url := serve(mustNotCache(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = fmt.Fprintf(w, "version %d", version.Add(1))
		}))
		rt := httpcache.NewETagCache(http.DefaultTransport)

		_, first := doGetRequest(rt, url)
		resp, second := doGetRequest(rt, url)

		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect([]string{first, second}).To(Equal([]string{"version 1", "version 2"}))
	})

	It("does not cache a non-200 response", func() {
		url := serve(mustNotCache(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("ETag", `"v1"`)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		}))
		rt := httpcache.NewETagCache(http.DefaultTransport)

		for range 2 {
			resp, body := doGetRequest(rt, url)
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
			Expect(body).To(Equal("boom"))
		}
	})

	It("passes non-GET requests straight through", func() {
		url := serve(mustNotCache(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write([]byte("payload"))
		}))
		rt := httpcache.NewETagCache(http.DefaultTransport)

		for range 2 {
			resp, body := doRequest(rt, http.MethodPost, url)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(body).To(Equal("payload"))
		}
	})

	It("caches each URL separately", func() {
		url := serve(mustCache())
		rt := httpcache.NewETagCache(http.DefaultTransport)

		doGetRequest(rt, url+"/a")
		doGetRequest(rt, url+"/b")
		respA, a := doGetRequest(rt, url+"/a")
		respB, b := doGetRequest(rt, url+"/b")

		Expect(respA.StatusCode).To(Equal(http.StatusOK))
		Expect(respB.StatusCode).To(Equal(http.StatusOK))
		Expect(a).To(Equal("contents of /a"))
		Expect(b).To(Equal("contents of /b"), "one entry per URL, or /b would revalidate with /a's ETag")
	})

	It("falls back to the default transport when next is nil", func() {
		url := serve(mustCache())
		rt := httpcache.NewETagCache(nil)

		_, first := doGetRequest(rt, url)
		_, second := doGetRequest(rt, url)

		Expect(first).To(Equal("contents of /"))
		Expect(second).To(Equal(first))
	})

	It("drops the hop-by-hop headers of the 304 exchange", func() {
		url := serve(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("ETag", `"v1"`)
			if r.Header.Get("If-None-Match") == `"v1"` {
				// These describe the connection the 304 arrived on, not the
				// representation being replayed.
				w.Header().Set("Connection", "keep-alive")
				w.Header().Set("Keep-Alive", "timeout=5")
				w.WriteHeader(http.StatusNotModified)
				return
			}
			_, _ = w.Write([]byte("payload"))
		})
		rt := httpcache.NewETagCache(http.DefaultTransport)

		doGetRequest(rt, url)
		resp, body := doGetRequest(rt, url)

		Expect(body).To(Equal("payload"))
		Expect(resp.Header).NotTo(HaveKey("Connection"))
		Expect(resp.Header).NotTo(HaveKey("Keep-Alive"))
	})

	It("reports a transport failure and keeps the entry it already has", func() {
		url := serve(mustCache())
		offline := errors.New("dial tcp: connection refused")
		var down atomic.Bool
		rt := httpcache.NewETagCache(transportFunc(func(r *http.Request) (*http.Response, error) {
			if down.Load() {
				return nil, offline
			}
			return http.DefaultTransport.RoundTrip(r)
		}))

		_, first := doGetRequest(rt, url)
		down.Store(true)
		resp, err := tryGet(rt, url)
		down.Store(false)
		_, afterwards := doGetRequest(rt, url)

		Expect(first).To(Equal("contents of /"))
		Expect(resp).To(BeNil())
		Expect(err).To(MatchError(offline))
		Expect(afterwards).To(Equal(first), "a failed revalidation must not drop the entry; mustCache rejects a second unconditional GET")
	})

	It("reports a body it cannot read instead of caching half of it", func() {
		truncated := errors.New("connection reset by peer")
		rt := httpcache.NewETagCache(transportFunc(func(*http.Request) (*http.Response, error) {
			return cacheable(&faultyBody{readErr: truncated}), nil
		}))

		resp, err := tryGet(rt, "http://cache.test/")

		Expect(resp).To(BeNil())
		Expect(err).To(MatchError(truncated))
	})

	It("still answers the caller when the body cannot be closed, but stores nothing", func() {
		var version atomic.Int32
		rt := httpcache.NewETagCache(transportFunc(func(r *http.Request) (*http.Response, error) {
			if _, conditional := requestETag(r); conditional {
				return notModified(), nil
			}
			body := fmt.Sprintf("version %d", version.Add(1))
			return cacheable(&faultyBody{
				Reader:   strings.NewReader(body),
				closeErr: errors.New("broken pipe"),
			}), nil
		}))

		_, first := doGetRequest(rt, "http://cache.test/")
		_, second := doGetRequest(rt, "http://cache.test/")

		Expect([]string{first, second}).To(Equal([]string{"version 1", "version 2"}),
			"a stored entry would have made the second GET conditional and replayed version 1")
	})

	It("is safe to use from concurrent requests", func() {
		// pollRuns fans out one request per repo on a shared client.
		url := serve(mustCache())
		rt := httpcache.NewETagCache(http.DefaultTransport)

		var wg sync.WaitGroup
		for i := range 128 {
			wg.Go(func() {
				defer GinkgoRecover()
				path := fmt.Sprintf("/repo%d", i)
				doGetRequest(rt, url+path)
				_, body := doGetRequest(rt, url+path)
				Expect(body).To(Equal("contents of " + path))
			})
		}
		wg.Wait()
	})
})

// serve runs handler on a live server for the duration of the spec and returns
// its URL.
func serve(handler http.HandlerFunc) string {
	GinkgoHelper()
	srv := httptest.NewServer(handler)
	DeferCleanup(srv.Close)
	return srv.URL
}

// mustCache returns a handler for a resource the transport has to cache: its
// body, "contents of <path>", is served at most once per URL path, under an
// ETag of its own. After that only revalidation works: a request carrying that
// ETag gets a 304, anything else a 400. A refetch is therefore visible in the
// response itself, so no spec has to inspect what the transport sent.
func mustCache() http.HandlerFunc {
	return mustCachePadded(0)
}

// mustCachePadded is mustCache with extra bytes appended to every body, for
// exercising responses too large to be framed in a single chunk.
func mustCachePadded(extra int) http.HandlerFunc {
	var mu sync.Mutex
	issued := map[string]string{} // URL path -> the ETag handed out for it

	return func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()

		path := r.URL.Path
		etag, served := issued[path]

		if inm, conditional := requestETag(r); conditional {
			if !served || inm != etag {
				http.Error(w, "If-None-Match "+inm+" was never issued for "+path, http.StatusBadRequest)
				return
			}
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		if served {
			http.Error(w, path+" was already served; revalidate with its ETag instead", http.StatusBadRequest)
			return
		}

		etag = `"` + path + `"`
		issued[path] = etag
		w.Header().Set("ETag", etag)
		_, _ = fmt.Fprintf(w, "contents of %s", path)
		_, _ = w.Write([]byte(strings.Repeat("x", extra)))
	}
}

func mustNotCache(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, conditional := requestETag(r); conditional {
			http.Error(w, "nothing here is cacheable", http.StatusBadRequest)
			return
		}
		handler(w, r)
	}
}

// Presence of the header is what counts, not its value: a cache that stored a
// response carrying no ETag sends an empty If-None-Match.
func requestETag(r *http.Request) (string, bool) {
	values, ok := r.Header["If-None-Match"]
	if !ok {
		return "", false
	}
	return strings.Join(values, ", "), true
}

// transportFunc is an http.RoundTripper for the failures a live server cannot
// be made to produce, such as a body that refuses to be read.
type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// faultyBody is a response body that fails at Read or at Close.
type faultyBody struct {
	io.Reader
	readErr  error
	closeErr error
}

func (b *faultyBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.Reader.Read(p)
}

func (b *faultyBody) Close() error { return b.closeErr }

// cacheable is the 200 a transportFunc hands back for a response the cache has
// every reason to store.
func cacheable(body io.ReadCloser) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Etag": []string{`"v1"`}},
		Body:       body,
	}
}

func notModified() *http.Response {
	return &http.Response{
		StatusCode: http.StatusNotModified,
		Header:     http.Header{"Etag": []string{`"v1"`}},
		Body:       http.NoBody,
	}
}

func doGetRequest(rt http.RoundTripper, url string) (*http.Response, string) {
	GinkgoHelper()
	return doRequest(rt, http.MethodGet, url)
}

// tryGet is doGetRequest for the specs where the round trip is meant to fail.
func tryGet(rt http.RoundTripper, url string) (*http.Response, error) {
	GinkgoHelper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	Expect(err).NotTo(HaveOccurred())
	return rt.RoundTrip(req)
}

func doRequest(rt http.RoundTripper, method, url string) (*http.Response, string) {
	GinkgoHelper()
	req, err := http.NewRequestWithContext(context.Background(), method, url, nil)
	Expect(err).NotTo(HaveOccurred())
	resp, err := rt.RoundTrip(req)
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	return resp, string(body)
}
