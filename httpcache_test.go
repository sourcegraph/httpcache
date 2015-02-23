package httpcache

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	. "gopkg.in/check.v1"
)

var _ = fmt.Print

func Test(t *testing.T) { TestingT(t) }

type S struct {
	server    *httptest.Server
	client    http.Client
	transport *Transport
}

type fakeClock struct {
	elapsed time.Duration
}

func (c *fakeClock) since(t time.Time) time.Duration {
	return c.elapsed
}

var _ = Suite(&S{})

func (s *S) SetUpSuite(c *C) {
	t := NewMemoryCacheTransport()
	client := http.Client{Transport: t}
	s.transport = t
	s.client = client

	mux := http.NewServeMux()
	s.server = httptest.NewServer(mux)

	mux.HandleFunc("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
	}))

	mux.HandleFunc("/nostore", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
	}))

	mux.HandleFunc("/etag", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etag := "124567"
		if r.Header.Get("if-none-match") == etag {
			w.WriteHeader(http.StatusNotModified)
		}
		w.Header().Set("etag", etag)
	}))

	mux.HandleFunc("/lastmodified", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lm := "Fri, 14 Dec 2010 01:01:50 GMT"
		if r.Header.Get("if-modified-since") == lm {
			w.WriteHeader(http.StatusNotModified)
		}
		w.Header().Set("last-modified", lm)
	}))

	mux.HandleFunc("/varyaccept", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Vary", "Accept")
		w.Write([]byte("Some text content"))
	}))

	mux.HandleFunc("/doublevary", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Vary", "Accept, Accept-Language")
		w.Write([]byte("Some text content"))
	}))
	mux.HandleFunc("/2varyheaders", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Add("Vary", "Accept")
		w.Header().Add("Vary", "Accept-Language")
		w.Write([]byte("Some text content"))
	}))
	mux.HandleFunc("/varyunused", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Vary", "X-Madeup-Header")
		w.Write([]byte("Some text content"))
	}))
	mux.HandleFunc("/ranged", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		testData := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Header().Set("Content-Type", "text/plain")
		start, end, err := findRanges(r, int64(len(testData)))
		if err == nil {
			w.Header().Set("content-range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(testData)))
			w.Write([]byte(testData)[start:end])
		} else {
			w.Write([]byte(testData))
		}
	}))

	updateFieldsCounter := 0
	mux.HandleFunc("/updatefields", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Counter", strconv.Itoa(updateFieldsCounter))
		w.Header().Set("Etag", `"e"`)
		updateFieldsCounter++
		if r.Header.Get("if-none-match") != "" {
			w.WriteHeader(http.StatusNotModified)
		} else {
			w.Write([]byte("Some text content"))
		}
	}))
}

func (s *S) TearDownSuite(c *C) {
	s.server.Close()
}

func (s *S) TearDownTest(c *C) {
	s.transport.Cache = NewMemoryCache()
	clock = &realClock{}
}

func (s *S) TestSuffixRangedQuery(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/ranged", nil)
	c.Assert(err, IsNil)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(len(data), Equals, 52)
	c.Assert(string(data), Equals, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	c.Assert(resp.Header.Get(XFromCache), Equals, "")

	req.Header.Add("Range", "bytes=10-")
	resp2, err := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")
	c.Assert(err, IsNil)
	data2, err := ioutil.ReadAll(resp2.Body)
	c.Assert(len(data2), Equals, 42)
	c.Assert(string(data2), Equals, "KLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	c.Assert(resp2.Header.Get("content-range"), Equals, "bytes 10-52/52")
}

func (s *S) TestPrefixRangedQuery(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/ranged", nil)
	c.Assert(err, IsNil)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	c.Assert(len(data), Equals, 52)
	c.Assert(string(data), Equals, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
	c.Assert(resp.Header.Get("content-range"), Equals, "")
	c.Assert(resp.Header.Get(XFromCache), Equals, "")

	req.Header.Add("Range", "bytes=-10")
	resp2, err := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")
	c.Assert(err, IsNil)
	data2, err := ioutil.ReadAll(resp2.Body)
	c.Assert(err, IsNil)
	c.Assert(len(data2), Equals, 10)
	c.Assert(string(data2), Equals, "qrstuvwxyz")
	c.Assert(resp2.Header.Get("content-range"), Equals, "bytes 42-52/52")
}

func (s *S) TestCompleteRangedQuery(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/ranged", nil)
	c.Assert(err, IsNil)
	req.Header.Add("Range", "bytes=0-10")
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	c.Assert(len(data), Equals, 10)
	c.Assert(string(data), Equals, "ABCDEFGHIJ")
	resp2, err := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")
	c.Assert(err, IsNil)
	data2, err := ioutil.ReadAll(resp2.Body)
	c.Assert(len(data2), Equals, 10)
	c.Assert(string(data2), Equals, "ABCDEFGHIJ")
	c.Assert(resp2.Header.Get("content-range"), Equals, "bytes 0-10/10")
}

func (s *S) TestPartialSubrangeRangedQuery(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/ranged", nil)
	c.Assert(err, IsNil)
	req.Header.Add("Range", "bytes=0-10")
	resp, err := s.client.Do(req)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(len(data), Equals, 10)
	c.Assert(string(data), Equals, "ABCDEFGHIJ")
	c.Assert(resp.Header.Get("content-range"), Equals, "bytes 0-10/52")

	req2, err := http.NewRequest("GET", s.server.URL+"/ranged", nil)
	c.Assert(err, IsNil)
	req2.Header.Add("Range", "bytes=4-6")
	resp2, err := s.client.Do(req2)
	c.Assert(err, IsNil)
	defer resp2.Body.Close()
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")
	data2, err := ioutil.ReadAll(resp2.Body)
	c.Assert(err, IsNil)
	c.Assert(len(data2), Equals, 2)
	c.Assert(string(data2), Equals, "EF")
	c.Assert(resp2.Header.Get("content-range"), Equals, "bytes 4-6/10")

	// test failing subrange outside previously held one
	req3, err := http.NewRequest("GET", s.server.URL+"/ranged", nil)
	c.Assert(err, IsNil)
	req3.Header.Add("Range", "bytes=8-15")
	resp3, err := s.client.Do(req3)
	defer resp3.Body.Close()
	c.Assert(resp3.Header.Get(XFromCache), Equals, "")
	c.Assert(err, IsNil)
	data3, err := ioutil.ReadAll(resp3.Body)
	c.Assert(err, IsNil)
	c.Assert(len(data3), Equals, 7)
	c.Assert(string(data3), Equals, "IJKLMNO")
	c.Assert(resp3.Header.Get("content-range"), Equals, "bytes 8-15/52")
}

func (s *S) TestMultipleSubrangeRangedQuery(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/ranged", nil)
	c.Assert(err, IsNil)
	req.Header.Add("Range", "bytes=0-10,15-40")
	resp, err := s.client.Do(req)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	c.Assert(err, IsNil)
	c.Assert(len(data), Equals, 52)
	c.Assert(string(data), Equals, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")
}

func (s *S) TestGetOnlyIfCachedHit(c *C) {
	req, err := http.NewRequest("GET", s.server.URL, nil)
	c.Assert(err, IsNil)
	resp, err := s.client.Do(req)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	c.Assert(resp.Header.Get(XFromCache), Equals, "")

	req2, err2 := http.NewRequest("GET", s.server.URL, nil)
	req2.Header.Add("cache-control", "only-if-cached")
	c.Assert(err2, IsNil)
	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(err2, IsNil)
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")
	c.Assert(resp2.StatusCode, Equals, 200)
}

func (s *S) TestGetOnlyIfCachedMiss(c *C) {
	req, err := http.NewRequest("GET", s.server.URL, nil)
	c.Assert(err, IsNil)
	req.Header.Add("cache-control", "only-if-cached")
	resp, err := s.client.Do(req)
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	c.Assert(resp.Header.Get(XFromCache), Equals, "")
	c.Assert(resp.StatusCode, Equals, 504)
}

func (s *S) TestGetNoStoreRequest(c *C) {
	req, err := http.NewRequest("GET", s.server.URL, nil)
	c.Assert(err, IsNil)
	req.Header.Add("Cache-Control", "no-store")
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	c.Assert(resp.Header.Get(XFromCache), Equals, "")

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(err2, IsNil)
	c.Assert(resp2.Header.Get(XFromCache), Equals, "")
}

func (s *S) TestGetNoStoreResponse(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/nostore", nil)
	c.Assert(err, IsNil)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	c.Assert(resp.Header.Get(XFromCache), Equals, "")

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(err2, IsNil)
	c.Assert(resp2.Header.Get(XFromCache), Equals, "")
}

func (s *S) TestGetWithEtag(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/etag", nil)
	c.Assert(err, IsNil)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	c.Assert(resp.Header.Get(XFromCache), Equals, "")

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(err2, IsNil)
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")

	// additional assertions to verify that 304 response is converted properly
	c.Assert(resp2.Status, Equals, "200 OK")
	_, ok := resp2.Header["Connection"]
	c.Assert(ok, Equals, false)
}

func (s *S) TestGetWithLastModified(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/lastmodified", nil)
	c.Assert(err, IsNil)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	c.Assert(resp.Header.Get(XFromCache), Equals, "")

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(err2, IsNil)
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")
}

func (s *S) TestGetWithVary(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/varyaccept", nil)
	c.Assert(err, IsNil)
	req.Header.Set("Accept", "text/plain")
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	c.Assert(resp.Header.Get("Vary"), Equals, "Accept")

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(err2, IsNil)
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")

	req.Header.Set("Accept", "text/html")
	resp3, err3 := s.client.Do(req)
	defer resp3.Body.Close()
	c.Assert(err3, IsNil)
	c.Assert(resp3.Header.Get(XFromCache), Equals, "")

	req.Header.Set("Accept", "")
	resp4, err4 := s.client.Do(req)
	defer resp4.Body.Close()
	c.Assert(err4, IsNil)
	c.Assert(resp4.Header.Get(XFromCache), Equals, "")
}

func (s *S) TestGetWithDoubleVary(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/doublevary", nil)
	c.Assert(err, IsNil)
	req.Header.Set("Accept", "text/plain")
	req.Header.Set("Accept-Language", "da, en-gb;q=0.8, en;q=0.7")
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	c.Assert(resp.Header.Get("Vary"), Not(Equals), "")

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(err2, IsNil)
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")

	req.Header.Set("Accept-Language", "")
	resp3, err3 := s.client.Do(req)
	defer resp3.Body.Close()
	c.Assert(err3, IsNil)
	c.Assert(resp3.Header.Get(XFromCache), Equals, "")

	req.Header.Set("Accept-Language", "da")
	resp4, err4 := s.client.Do(req)
	defer resp4.Body.Close()
	c.Assert(err4, IsNil)
	c.Assert(resp4.Header.Get(XFromCache), Equals, "")
}

func (s *S) TestGetWith2VaryHeaders(c *C) {
	// Tests that multiple Vary headers' comma-separated lists are
	// merged. See https://github.com/gregjones/httpcache/issues/27.
	const (
		accept         = "text/plain"
		acceptLanguage = "da, en-gb;q=0.8, en;q=0.7"
	)
	req, err := http.NewRequest("GET", s.server.URL+"/2varyheaders", nil)
	c.Assert(err, IsNil)
	req.Header.Set("Accept", accept)
	req.Header.Set("Accept-Language", acceptLanguage)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	c.Assert(resp.Header.Get("Vary"), Not(Equals), "")

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(err2, IsNil)
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")

	req.Header.Set("Accept-Language", "")
	resp3, err3 := s.client.Do(req)
	defer resp3.Body.Close()
	c.Assert(err3, IsNil)
	c.Assert(resp3.Header.Get(XFromCache), Equals, "")

	req.Header.Set("Accept-Language", "da")
	resp4, err4 := s.client.Do(req)
	defer resp4.Body.Close()
	c.Assert(err4, IsNil)
	c.Assert(resp4.Header.Get(XFromCache), Equals, "")

	req.Header.Set("Accept-Language", acceptLanguage)
	req.Header.Set("Accept", "")
	resp5, err5 := s.client.Do(req)
	defer resp5.Body.Close()
	c.Assert(err5, IsNil)
	c.Assert(resp5.Header.Get(XFromCache), Equals, "")

	req.Header.Set("Accept", "image/png")
	resp6, err6 := s.client.Do(req)
	defer resp6.Body.Close()
	c.Assert(err6, IsNil)
	c.Assert(resp6.Header.Get(XFromCache), Equals, "")

	resp7, err7 := s.client.Do(req)
	defer resp7.Body.Close()
	c.Assert(err7, IsNil)
	c.Assert(resp7.Header.Get(XFromCache), Equals, "1")
}

func (s *S) TestGetVaryUnused(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/varyunused", nil)
	c.Assert(err, IsNil)
	req.Header.Set("Accept", "text/plain")
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	c.Assert(resp.Header.Get("Vary"), Not(Equals), "")

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(err2, IsNil)
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")
}

func (s *S) TestUpdateFields(c *C) {
	req, err := http.NewRequest("GET", s.server.URL+"/updatefields", nil)
	c.Assert(err, IsNil)
	resp, err := s.client.Do(req)
	defer resp.Body.Close()
	c.Assert(err, IsNil)
	counter := resp.Header.Get("x-counter")

	resp2, err2 := s.client.Do(req)
	defer resp2.Body.Close()
	c.Assert(err2, IsNil)
	c.Assert(resp2.Header.Get(XFromCache), Equals, "1")
	counter2 := resp2.Header.Get("x-counter")

	c.Assert(counter, Not(Equals), counter2)
}

func (s *S) TestParseCacheControl(c *C) {
	h := http.Header{}
	for _ = range parseCacheControl(h) {
		c.Fatal("cacheControl should be empty")
	}

	h.Set("cache-control", "no-cache")
	cc := parseCacheControl(h)
	if _, ok := cc["foo"]; ok {
		c.Error("Value shouldn't exist")
	}
	if nocache, ok := cc["no-cache"]; ok {
		c.Assert(nocache, Equals, "")
	}

	h.Set("cache-control", "no-cache, max-age=3600")
	cc = parseCacheControl(h)
	c.Assert(cc["no-cache"], Equals, "")
	c.Assert(cc["max-age"], Equals, "3600")
}

func (s *S) TestNoCacheRequestExpiration(c *C) {
	respHeaders := http.Header{}
	respHeaders.Set("Cache-Control", "max-age=7200")
	reqHeaders := http.Header{}
	reqHeaders.Set("Cache-Control", "no-cache")

	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, transparent)
}

func (s *S) TestNoCacheResponseExpiration(c *C) {
	respHeaders := http.Header{}
	respHeaders.Set("Cache-Control", "no-cache")
	respHeaders.Set("Expires", "Wed, 19 Apr 3000 11:43:00 GMT")
	reqHeaders := http.Header{}

	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, stale)
}

func (s *S) TestReqMustRevalidate(c *C) {
	// not paying attention to request setting max-stale means never returning stale
	// responses, so always acting as if must-revalidate is set
	respHeaders := http.Header{}
	reqHeaders := http.Header{}
	reqHeaders.Set("Cache-Control", "must-revalidate")

	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, stale)
}

func (s *S) TestRespMustRevalidate(c *C) {
	respHeaders := http.Header{}
	respHeaders.Set("Cache-Control", "must-revalidate")
	reqHeaders := http.Header{}

	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, stale)
}

func (s *S) TestFreshExpiration(c *C) {
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("expires", now.Add(time.Duration(2)*time.Second).Format(time.RFC1123))

	reqHeaders := http.Header{}
	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, fresh)

	clock = &fakeClock{elapsed: 3 * time.Second}
	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, stale)
}

func (s *S) TestMaxAge(c *C) {
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=2")

	reqHeaders := http.Header{}
	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, fresh)

	clock = &fakeClock{elapsed: 3 * time.Second}
	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, stale)
}

func (s *S) TestMaxAgeZero(c *C) {
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=0")

	reqHeaders := http.Header{}
	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, stale)
}

func (s *S) TestBothMaxAge(c *C) {
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=2")

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "max-age=0")
	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, stale)
}

func (s *S) TestMinFreshWithExpires(c *C) {
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("expires", now.Add(time.Duration(2)*time.Second).Format(time.RFC1123))

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "min-fresh=1")
	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, fresh)

	reqHeaders = http.Header{}
	reqHeaders.Set("cache-control", "min-fresh=2")
	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, stale)
}

func (s *S) TestEmptyMaxStale(c *C) {
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=20")

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "max-stale")

	clock = &fakeClock{elapsed: 10 * time.Second}

	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, fresh)

	clock = &fakeClock{elapsed: 60 * time.Second}

	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, fresh)
}

func (s *S) TestMaxStaleValue(c *C) {
	now := time.Now()
	respHeaders := http.Header{}
	respHeaders.Set("date", now.Format(time.RFC1123))
	respHeaders.Set("cache-control", "max-age=10")

	reqHeaders := http.Header{}
	reqHeaders.Set("cache-control", "max-stale=20")
	clock = &fakeClock{elapsed: 5 * time.Second}

	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, fresh)

	clock = &fakeClock{elapsed: 15 * time.Second}

	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, fresh)

	clock = &fakeClock{elapsed: 30 * time.Second}

	c.Assert(getFreshness(respHeaders, reqHeaders), Equals, stale)
}

func containsHeader(headers []string, header string) bool {
	for _, v := range headers {
		if http.CanonicalHeaderKey(v) == http.CanonicalHeaderKey(header) {
			return true
		}
	}
	return false
}

type containsHeaderChecker struct {
	*CheckerInfo
}

func (c *containsHeaderChecker) Check(params []interface{}, names []string) (bool, string) {
	items, ok := params[0].([]string)
	if !ok {
		return false, "Expected first param to be []string"
	}
	value, ok := params[1].(string)
	if !ok {
		return false, "Expected 2nd param to be string"
	}
	return containsHeader(items, value), ""
}

var ContainsHeader Checker = &containsHeaderChecker{&CheckerInfo{Name: "Contains", Params: []string{"Container", "expected to contain"}}}

func (s *S) TestGetEndToEndHeaders(c *C) {
	var (
		headers http.Header
		end2end []string
	)

	headers = http.Header{}
	headers.Set("content-type", "text/html")
	headers.Set("te", "deflate")

	end2end = getEndToEndHeaders(headers)
	c.Check(end2end, ContainsHeader, "content-type")
	c.Check(end2end, Not(ContainsHeader), "te")

	headers = http.Header{}
	headers.Set("connection", "content-type")
	headers.Set("content-type", "text/csv")
	headers.Set("te", "deflate")
	end2end = getEndToEndHeaders(headers)
	c.Check(end2end, Not(ContainsHeader), "connection")
	c.Check(end2end, Not(ContainsHeader), "content-type")
	c.Check(end2end, Not(ContainsHeader), "te")

	headers = http.Header{}
	end2end = getEndToEndHeaders(headers)
	c.Check(end2end, HasLen, 0)

	headers = http.Header{}
	headers.Set("connection", "content-type")
	end2end = getEndToEndHeaders(headers)
	c.Check(end2end, HasLen, 0)
}
