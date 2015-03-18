package cacheproxy

import (
	"github.com/uovobw/httpcache"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

var (
	memoryCache = httpcache.NewMemoryCache()
	transport   = httpcache.NewTransport(memoryCache)
)

// NewSingleHostReverseProxy wraps net/http/httputil.NewSingleHostReverseProxy
// and sets the Host header based on the target URL.
func NewSingleHostReverseProxy(url *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(url)
	oldDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		oldDirector(r)
		r.Host = url.Host
	}
	proxy.Transport = transport
	return proxy
}

// SetLogger wraps httpcache.SetLogger
func SetLogger(l *log.Logger) {
	transport.SetLogger(l)
}
