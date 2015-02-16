package httpcache

import (
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gregjones/httpcache"
	"github.com/gregjones/httpcache/diskcache"
)

func TestRaceCondition0(t *testing.T) {
	t0 := time.Now()
	wantBody := strings.Repeat("abc", 10000)
	numResps := 0
	var numRespsMu sync.Mutex

	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("last-modified", t0.Format(http.TimeFormat))
		w.Header().Set("cache-control", "max-age=60")
		w.Write([]byte(wantBody))
		numRespsMu.Lock()
		numResps++
		numRespsMu.Unlock()
	}))

	s := httptest.NewServer(mux)
	defer s.Close()

	var cacheTransport http.RoundTripper
	if true {
		tmpDir, err := ioutil.TempDir("", "httpcache-race-cond")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(tmpDir)
		cacheTransport = &httpcache.Transport{Cache: diskcache.New(tmpDir)}
	} else {
		cacheTransport = NewMemoryCacheTransport()
	}
	httpClient := &http.Client{Transport: cacheTransport}

	var wg sync.WaitGroup
	const (
		c = 100 // number of concurrent workers
		n = 100 // number of HTTP GETs to issue
	)
	for i := 0; i < c; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < n; j++ {
				resp, err := httpClient.Get(s.URL)
				if err != nil {
					t.Errorf("worker %d req %d: Get error: %s", i, j, err)
					break
				}
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					t.Errorf("worker %d req %d: ReadAll error: %s", i, j, err)
					continue
				}
				if err := resp.Body.Close(); err != nil {
					t.Errorf("worker %d req %d: close body error: %s", i, j, err)
					continue
				}
				if string(body) != wantBody {
					t.Errorf("worker %d req %d: got body %q, want %q", i, j, body, wantBody)
				}
			}
		}()
	}
	wg.Wait()

	t.Logf("served %d responses", numResps)
}
