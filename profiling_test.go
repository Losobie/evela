package main

import (
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestPerformanceProfilerDisabledAndLoopbackOnly(t *testing.T) {
	stop, err := startPerformanceProfiler("")
	if err != nil || stop == nil {
		t.Fatal("disabled profiler failed")
	}
	stop()
	for _, address := range []string{"0.0.0.0:6060", "192.168.1.1:6060", "[::]:6060", "localhost:6060", "invalid"} {
		if stop, err := startPerformanceProfiler(address); err == nil {
			stop()
			t.Fatalf("accepted %s", address)
		}
	}
}

func TestPerformanceProfilerServesProfilesAndStops(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	stop, err := startPerformanceProfiler(address)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	client := &http.Client{Timeout: 3 * time.Second}
	for _, path := range []string{"/debug/pprof/", "/debug/pprof/heap", "/debug/pprof/goroutine?debug=1", "/debug/pprof/trace?seconds=0.01"} {
		response, err := client.Get("http://" + address + path)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || len(data) == 0 {
			t.Fatalf("%s: status %d error %v", path, response.StatusCode, err)
		}
		if path == "/debug/pprof/" && !strings.Contains(string(data), "heap") {
			t.Fatal("profile index missing")
		}
	}
	stop()
	if response, err := client.Get("http://" + address + "/debug/pprof/"); err == nil {
		response.Body.Close()
		t.Fatal("profiler listener survived shutdown")
	}
}
