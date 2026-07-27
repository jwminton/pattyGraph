package main

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestParseServerOptions(t *testing.T) {
	var stderr bytes.Buffer

	options, err := parseServerOptions(nil, &stderr)
	if err != nil {
		t.Fatalf("parse default options: %v", err)
	}
	if options.listenAddress != defaultListenAddress {
		t.Fatalf("default listen address = %q, want %q", options.listenAddress, defaultListenAddress)
	}

	options, err = parseServerOptions([]string{"--listen", "127.0.0.1:0"}, &stderr)
	if err != nil {
		t.Fatalf("parse custom options: %v", err)
	}
	if options.listenAddress != "127.0.0.1:0" {
		t.Fatalf("custom listen address = %q", options.listenAddress)
	}
}

func TestParseServerOptionsRejectsPositionalArguments(t *testing.T) {
	_, err := parseServerOptions([]string{"traffic.jsonl"}, &bytes.Buffer{})
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

	assetPattern := regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`)
	assets := assetPattern.FindAllStringSubmatch(body, -1)
	if len(assets) < 2 {
		t.Fatalf("index referenced %d assets, want at least JavaScript and CSS", len(assets))
	}
	for _, asset := range assets {
		response := requestEmbedded(t, handler, http.MethodGet, asset[1], http.StatusOK)
		contentType := response.Header().Get("Content-Type")
		switch {
		case strings.HasSuffix(asset[1], ".js") && !strings.Contains(contentType, "javascript"):
			t.Errorf("JavaScript asset %q content type = %q", asset[1], contentType)
		case strings.HasSuffix(asset[1], ".css") && !strings.Contains(contentType, "text/css"):
			t.Errorf("CSS asset %q content type = %q", asset[1], contentType)
		}
	}

	workers, err := fs.Glob(embeddedDist, "dist/assets/*.worker-*.js")
	if err != nil {
		t.Fatalf("find embedded worker: %v", err)
	}
	if len(workers) == 0 {
		t.Fatal("embedded build does not contain the PattyLog parser worker")
	}
	for _, worker := range workers {
		requestEmbedded(t, handler, http.MethodGet, strings.TrimPrefix(worker, "dist"), http.StatusOK)
	}
}

func TestEmbeddedHandlerSupportsHeadAndRejectsOtherMethods(t *testing.T) {
	handler := mustEmbeddedHandler(t)
	requestEmbedded(t, handler, http.MethodHead, "/", http.StatusOK)

	response := requestEmbedded(t, handler, http.MethodPost, "/", http.StatusMethodNotAllowed)
	if response.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("Allow header = %q", response.Header().Get("Allow"))
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
