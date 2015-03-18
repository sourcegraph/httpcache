package main

import (
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"

	"github.com/gorilla/handlers"
	"github.com/uovobw/httpcache/cacheproxy"
)

var (
	bindTo = flag.String("bind", "0.0.0.0:8080", "address to bind to")
	debug  = flag.Bool("debug", false, "enable debugging")
	target = flag.String("target", "", "base url to cache")
)

func init() {
	flag.Parse()
	flag.VisitAll(func(f *flag.Flag) {
		log.Printf("%s=%v", f.Name, f.Value)
	})
	if *target == "" {
		log.Fatalln("you must specify a target url")
	}
	if *debug {
		logger := log.New(os.Stdout, "", 0)
		cacheproxy.SetLogger(logger)
	}
}

func main() {
	URL, err := url.Parse(*target)
	if err != nil {
		log.Fatal(err)
	}
	proxy := cacheproxy.NewSingleHostReverseProxy(URL)
	log.Fatal(http.ListenAndServe(*bindTo, handlers.CombinedLoggingHandler(os.Stdout, proxy)))
}
