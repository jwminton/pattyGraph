package main

import (
	"bytes"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseApplicationOptions(t *testing.T) {
	var stderr bytes.Buffer

	options, err := parseApplicationOptions(nil, &stderr)
	if err != nil {
		t.Fatalf("parse default options: %v", err)
	}
	if options.listenAddress != defaultListenAddress {
		t.Fatalf("default listen address = %q, want %q", options.listenAddress, defaultListenAddress)
	}

	options, err = parseApplicationOptions([]string{"--listen", "127.0.0.1:0"}, &stderr)
	if err != nil {
		t.Fatalf("parse custom options: %v", err)
	}
	if options.listenAddress != "127.0.0.1:0" {
		t.Fatalf("custom listen address = %q", options.listenAddress)
	}

	options, err = parseApplicationOptions([]string{"-l", "127.0.0.1:4180"}, &stderr)
	if err != nil {
		t.Fatalf("parse shorthand listen option: %v", err)
	}
	if options.listenAddress != "127.0.0.1:4180" {
		t.Fatalf("shorthand listen address = %q", options.listenAddress)
	}
}

func TestHelpGroupsOptionAliases(t *testing.T) {
	var stderr bytes.Buffer
	_, err := parseApplicationOptions([]string{"--help"}, &stderr)
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("parse help error = %v, want flag.ErrHelp", err)
	}
	for _, line := range []string{
		"-l, --listen address",
		"-v, --version",
		"--bundle PattyLog",
		"--from log-time",
		"--through log-time",
		"-h, --help",
	} {
		if !strings.Contains(stderr.String(), line) {
			t.Errorf("help does not group %q:\n%s", line, stderr.String())
		}
	}
}

func TestRunPrintsVersionAndExits(t *testing.T) {
	for _, argument := range []string{"-v", "-version", "--version"} {
		t.Run(argument, func(t *testing.T) {
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			if err := run([]string{argument}, &stdout, &stderr); err != nil {
				t.Fatalf("run %s: %v", argument, err)
			}
			if got, want := stdout.String(), "pattyView "+PattyViewVersion+"\n"; got != want {
				t.Fatalf("version output = %q, want %q", got, want)
			}
			if stderr.Len() != 0 {
				t.Fatalf("version stderr = %q", stderr.String())
			}
		})
	}
}

func TestParseApplicationOptionsRejectsPositionalArguments(t *testing.T) {
	_, err := parseApplicationOptions([]string{"traffic.jsonl"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unexpected positional argument") {
		t.Fatalf("parse positional argument error = %v", err)
	}
}

func TestEmbeddedHandlerServesApplicationAssets(t *testing.T) {
	handler := mustEmbeddedHandler(t)
	index := requestEmbedded(t, handler, http.MethodGet, "/", http.StatusOK)
	if !strings.Contains(index.Header().Get("Content-Type"), "text/html") {
		t.Fatalf("index content type = %q", index.Header().Get("Content-Type"))
	}
	body := index.Body.String()
	if !strings.Contains(body, `<div id="app"></div>`) {
		t.Fatal("embedded index does not contain the PattyView application root")
	}
	assertNoStore(t, index)

	assets := []string{"/assets/pattyView.js", "/assets/pattyView.css"}
	for _, asset := range assets {
		if !strings.Contains(body, `"`+asset+`"`) {
			t.Errorf("embedded index does not reference %s", asset)
		}
		response := requestEmbedded(t, handler, http.MethodGet, asset, http.StatusOK)
		assertNoStore(t, response)
		contentType := response.Header().Get("Content-Type")
		switch {
		case strings.HasSuffix(asset, ".js") && !strings.Contains(contentType, "javascript"):
			t.Errorf("JavaScript asset %q content type = %q", asset, contentType)
		case strings.HasSuffix(asset, ".css") && !strings.Contains(contentType, "text/css"):
			t.Errorf("CSS asset %q content type = %q", asset, contentType)
		}
	}

	worker := requestEmbedded(t, handler, http.MethodGet, "/assets/jsonl.worker.js", http.StatusOK)
	assertNoStore(t, worker)
	if contentType := worker.Header().Get("Content-Type"); !strings.Contains(contentType, "javascript") {
		t.Errorf("worker content type = %q", contentType)
	}
}

func TestEmbeddedHandlerSupportsHeadAndRejectsOtherMethods(t *testing.T) {
	handler := mustEmbeddedHandler(t)
	head := requestEmbedded(t, handler, http.MethodHead, "/", http.StatusOK)
	assertNoStore(t, head)

	response := requestEmbedded(t, handler, http.MethodPost, "/", http.StatusMethodNotAllowed)
	if response.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("Allow header = %q", response.Header().Get("Allow"))
	}
}

func assertNoStore(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	if cacheControl := response.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", cacheControl)
	}
}

func TestEmbeddedHandlerReturnsNotFound(t *testing.T) {
	requestEmbedded(t, mustEmbeddedHandler(t), http.MethodGet, "/missing.js", http.StatusNotFound)
}

func mustEmbeddedHandler(t *testing.T) http.Handler {
	t.Helper()
	handler, err := embeddedHandler()
	if err != nil {
		t.Fatalf("build embedded handler: %v", err)
	}
	return handler
}

func requestEmbedded(t *testing.T, handler http.Handler, method, path string, status int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s %s status = %d, want %d", method, path, response.Code, status)
	}
	return response
}
