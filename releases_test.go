package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewerReleaseComparison(t *testing.T) {
	current, latest := strings.Repeat("a", 40), strings.Repeat("b", 40)
	for _, item := range []struct {
		name, tag, comparison string
		draft, prerelease     bool
		wantCompare, wantNew  bool
	}{
		{"newer", "build-" + latest, `{"status":"ahead","ahead_by":2}`, false, false, true, true},
		{"versioned newer", "v0.1.81", `{"status":"ahead","ahead_by":2}`, false, false, true, true},
		{"versioned same", "v0.1.81", `{"status":"identical","ahead_by":0}`, false, false, true, false},
		{"higher version older commit", "v1.0.0", `{"status":"behind","ahead_by":0}`, false, false, true, false},
		{"versioned diverged", "v0.1.81", `{"status":"diverged","ahead_by":2}`, false, false, true, false},
		{"versioned draft", "v0.1.81", "", true, false, false, false},
		{"versioned prerelease", "v0.1.81", "", false, true, false, false},
		{"same", "build-" + current, "", false, false, false, false},
		{"same uppercase", "build-" + strings.ToUpper(current), "", false, false, false, false},
		{"older republished", "build-" + latest, `{"status":"behind","ahead_by":0}`, false, false, true, false},
		{"diverged", "build-" + latest, `{"status":"diverged","ahead_by":2}`, false, false, true, false},
		{"identical comparison", "build-" + latest, `{"status":"identical","ahead_by":0}`, false, false, true, false},
		{"empty comparison", "build-" + latest, `{}`, false, false, true, false},
		{"draft", "build-" + latest, "", true, false, false, false},
		{"prerelease", "build-" + latest, "", false, true, false, false},
		{"unrecognized tag", "v1.0", "", false, false, false, false},
		{"leading zero version", "v0.1.081", "", false, false, false, false},
		{"unsafe tag", "v0.1.81/../../other", "", false, false, false, false},
		{"invalid revision", "build-" + strings.Repeat("z", 40), "", false, false, false, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			var comparisons atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "" || !strings.Contains(r.UserAgent(), "Evela") || r.Header.Get("Accept") != "application/vnd.github+json" {
					t.Error("release requests must identify Evela without sending credentials")
				}
				if r.URL.Path == "/releases/latest" {
					fmt.Fprintf(w, `{"tag_name":%q,"draft":%t,"prerelease":%t,"html_url":"https://untrusted.example/"}`, item.tag, item.draft, item.prerelease)
					return
				}
				comparisons.Add(1)
				target := latest
				if strings.HasPrefix(item.tag, "v") {
					target = item.tag
				}
				if r.URL.Path != "/compare/"+current+"..."+target || r.URL.Query().Get("per_page") != "1" {
					t.Errorf("unexpected comparison: %s", r.URL)
				}
				fmt.Fprint(w, item.comparison)
			}))
			defer server.Close()
			link, err := newerRelease(context.Background(), server.Client(), server.URL, current)
			if err != nil {
				t.Fatal(err)
			}
			var wantComparisons int32
			if item.wantCompare {
				wantComparisons = 1
			}
			if (link != nil) != item.wantNew || comparisons.Load() != wantComparisons {
				t.Fatalf("link=%v comparisons=%d; want new=%v comparisons=%d", link, comparisons.Load(), item.wantNew, wantComparisons)
			}
			if link != nil && link.String() != "https://github.com/Losobie/evela/releases/tag/"+item.tag {
				t.Fatalf("unexpected release URL: %s", link)
			}
		})
	}
}

func TestReleaseCheckFailures(t *testing.T) {
	for _, endpoint := range []string{"latest", "comparison"} {
		for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusOK} {
			t.Run(fmt.Sprintf("%s/%d", endpoint, status), func(t *testing.T) {
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if endpoint == "comparison" && r.URL.Path == "/releases/latest" {
						fmt.Fprintf(w, `{"tag_name":"build-%s"}`, strings.Repeat("b", 40))
						return
					}
					w.WriteHeader(status)
					fmt.Fprint(w, "not valid JSON")
				}))
				defer server.Close()
				link, err := newerRelease(context.Background(), server.Client(), server.URL, strings.Repeat("a", 40))
				if err == nil || link != nil {
					t.Fatalf("failed/invalid response must not advertise a release: %v, %v", link, err)
				}
				if errors.Is(err, errReleaseCheckUnavailable) != (status == http.StatusNotFound) {
					t.Fatalf("404 must be unavailable while genuine failures remain errors: %v", err)
				}
				if status != http.StatusOK && !strings.Contains(err.Error(), server.URL) {
					t.Fatal("HTTP failure omitted the failed endpoint")
				}
			})
		}
	}
}

func TestReleaseCheckCancellationAndUnknownBuild(t *testing.T) {
	// Unknown builds must not make an HTTP request at all.
	for _, revision := range []string{"", "devel", strings.Repeat("z", 40)} {
		if link, err := newerRelease(context.Background(), nil, "invalid endpoint", revision); link != nil || err != nil {
			t.Fatalf("unknown revision attempted a check: %v, %v", link, err)
		}
	}
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := newerRelease(ctx, server.Client(), server.URL, strings.Repeat("a", 40))
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("release check did not start")
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("request did not propagate cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("release check did not stop on cancellation")
	}
}
