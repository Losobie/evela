package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

const releaseAPI = "https://api.github.com/repos/Losobie/evela"
const releaseCheckInterval = 6 * time.Hour

func validBuildRevision(revision string) bool {
	if len(revision) != 40 {
		return false
	}
	_, err := hex.DecodeString(revision)
	return err == nil
}

func currentBuildRevision() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && validBuildRevision(setting.Value) {
				return strings.ToLower(setting.Value)
			}
		}
	}
	return ""
}

// Compare ancestry rather than publication time: republishing an old commit
// must not advertise a downgrade to users running a newer build.
func newerRelease(ctx context.Context, client *http.Client, api, revision string) (*url.URL, error) {
	if !validBuildRevision(revision) {
		return nil, nil // Builds without Git metadata cannot be compared reliably.
	}
	var release struct {
		TagName    string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := releaseJSON(ctx, client, api+"/releases/latest", &release); err != nil {
		return nil, err
	}
	latest, tagged := strings.CutPrefix(release.TagName, "build-")
	if !tagged || !validBuildRevision(latest) || release.Draft || release.Prerelease || strings.EqualFold(latest, revision) {
		return nil, nil
	}
	var comparison struct {
		Status  string `json:"status"`
		AheadBy int    `json:"ahead_by"`
	}
	if err := releaseJSON(ctx, client, api+"/compare/"+revision+"..."+latest+"?per_page=1", &comparison); err != nil {
		return nil, err
	}
	if comparison.Status != "ahead" || comparison.AheadBy <= 0 {
		return nil, nil
	}
	// Construct a repository-owned URL rather than opening a URL from the response.
	return url.Parse("https://github.com/Losobie/evela/releases/tag/" + release.TagName)
}

func releaseJSON(ctx context.Context, client *http.Client, endpoint string, result any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "Evela release check (https://github.com/Losobie/evela)")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("release check: GitHub returned HTTP %d", response.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(result)
}

func (ui *analyzerUI) watchReleases() {
	revision := currentBuildRevision()
	if revision == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-ui.stop:
			cancel()
		case <-ctx.Done():
		}
	}()
	client := &http.Client{Timeout: 10 * time.Second}
	ticker := time.NewTicker(releaseCheckInterval)
	defer ticker.Stop()
	for {
		checkCtx, stopCheck := context.WithTimeout(ctx, 10*time.Second)
		link, err := newerRelease(checkCtx, client, releaseAPI, revision)
		stopCheck()
		if ctx.Err() != nil {
			return
		}
		if err == nil {
			fyne.Do(func() {
				select {
				case <-ui.stop:
					return
				default:
				}
				ui.releaseLink.SetURL(link)
				if link != nil {
					ui.releaseLink.Show()
				} else {
					ui.releaseLink.Hide()
				}
			})
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
