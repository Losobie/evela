package main

import (
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"runtime"
)

// Profiling is opt-in and restricted to loopback. A normal application launch
// starts no diagnostics listener and enables no blocking/lock instrumentation.
func startPerformanceProfiler(address string) (func(), error) {
	if address == "" {
		return func() {}, nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("profiling address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("profiling address must use a loopback IP")
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	server := &http.Server{Handler: mux}
	runtime.SetBlockProfileRate(1_000_000)
	previousMutexRate := runtime.SetMutexProfileFraction(5)
	go server.Serve(listener)
	return func() {
		server.Close()
		runtime.SetBlockProfileRate(0)
		runtime.SetMutexProfileFraction(previousMutexRate)
	}, nil
}
