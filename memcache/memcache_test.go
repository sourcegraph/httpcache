// +build !appengine

package memcache

import (
	"bytes"
	"fmt"
	"net"
	"testing"
)

const testServer = "localhost:11211"

func SetUpSuite() {
	conn, err := net.Dial("tcp", testServer)
	if err != nil {
		// TODO: rather than skip the test, fall back to a faked memcached server
		fmt.Sprintf("skipping test; no server running at %s", testServer)
	}
	conn.Write([]byte("flush_all\r\n")) // flush memcache
	conn.Close()
}

func TestMemCache(t *testing.T) {
	SetUpSuite()
	cache := New(testServer)

	key := "testKey"
	_, ok := cache.Get(key)
	if ok != false {
		t.Fatal("retrieved key before adding it")
	}

	val := []byte("some bytes")
	cache.Set(key, val)

	retVal, ok := cache.Get(key)
	if ok != true {
		t.Fatal("could not retrieve an element i just added")
	}
	if bytes.Equal(retVal, val) != true {
		t.Fatal("retrieved a different thing than what i put in")
	}

	cache.Delete(key)

	_, ok = cache.Get(key)
	if ok != false {
		t.Fatal("deleted key still present")
	}
}
