// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"math"
	"reflect"
	"testing"
)

func ensureCommonWordsForTest() {
	if commonWords == nil {
		commonWords = makeCommonWordMap(commonWordList)
	}
}

func TestFastFieldsASCIIBuf(t *testing.T) {
	scratch := make([]string, 0, 1)

	got := fastFieldsASCIIBuf("  alpha  beta gamma  ", &scratch)
	want := []string{"alpha", "beta", "gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fastFieldsASCIIBuf() = %v, want %v", got, want)
	}

	got = fastFieldsASCIIBuf("", &scratch)
	if len(got) != 0 {
		t.Fatalf("fastFieldsASCIIBuf(empty) = %v, want empty slice", got)
	}
}

func TestTokensForRefs(t *testing.T) {
	got := append([]string(nil), tokensForRefs("example com a search terms")...)
	want := []string{"example", "com", "search", "terms"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokensForRefs() = %v, want %v", got, want)
	}
}

func TestTokensForIps(t *testing.T) {
	got := append([]string(nil), tokensForIps("192.0.2.10")...)
	want := []string{"192.0.2.10"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokensForIps() = %v, want %v", got, want)
	}
}

func TestLevenshteinTokensRatioUsesFirstSequenceAsBaseline(t *testing.T) {
	tests := []struct {
		name     string
		baseline []string
		current  []string
		want     float64
	}{
		{name: "equal", baseline: []string{"a", "b", "c"}, current: []string{"a", "b", "c"}, want: 0},
		{name: "substitution", baseline: []string{"a", "b", "c", "d"}, current: []string{"a", "b", "x", "d"}, want: 0.25},
		{name: "insertion", baseline: []string{"a", "b"}, current: []string{"a", "x", "b"}, want: 0.5},
		{name: "deletion", baseline: []string{"a", "b", "c"}, current: []string{"a", "c"}, want: 1.0 / 3.0},
		{name: "unrelated", baseline: []string{"a", "b", "c"}, current: []string{"x", "y", "z"}, want: 1},
		{name: "empty baseline", baseline: nil, current: []string{"a"}, want: 2},
		{name: "empty current", baseline: []string{"a"}, current: nil, want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := levenshteinTokensRatio(tt.baseline, tt.current)
			if math.Abs(got-tt.want) > 1e-12 {
				t.Fatalf("levenshteinTokensRatio() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTokensForWordsIncludesInterestingUserAgentAndRequestTokens(t *testing.T) {
	ensureCommonWordsForTest()
	PattyGraph = &Monitor{}
	*currentLine = lineSource{
		userAgentTokens: []string{"Mozilla", "SuspiciousBot", "42"},
	}

	got := append([]string(nil), tokensForWords("GET admin HTTP 200")...)
	want := []string{"SuspiciousBot", "admin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokensForWords() = %v, want %v", got, want)
	}
	if PattyGraph.totalAgentTokens != 1 {
		t.Fatalf("totalAgentTokens = %d, want 1", PattyGraph.totalAgentTokens)
	}
}

func TestTokensForWordsFallsBackWhenNothingInteresting(t *testing.T) {
	ensureCommonWordsForTest()
	PattyGraph = &Monitor{}
	*currentLine = lineSource{
		userAgentTokens: []string{"Mozilla", "42"},
	}

	got := append([]string(nil), tokensForWords("GET / HTTP")...)
	want := []string{filteredToken}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("tokensForWords() = %v, want %v", got, want)
	}
}

func TestIsInteresting(t *testing.T) {
	ensureCommonWordsForTest()

	tests := []struct {
		word string
		want bool
	}{
		{word: "ab", want: false},
		{word: "GET", want: false},
		{word: "123.45", want: false},
		{word: "SuspiciousBot", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			if got := isInteresting(tt.word); got != tt.want {
				t.Fatalf("isInteresting(%q) = %v, want %v", tt.word, got, tt.want)
			}
		})
	}
}

func TestClassifySizeBandKey(t *testing.T) {
	tests := []struct {
		bytes int
		want  string
	}{
		{bytes: 99, want: "<100B"},
		{bytes: 100, want: "100–300B"},
		{bytes: 699, want: "300–700B"},
		{bytes: 700, want: "700B–1K"},
		{bytes: 1024, want: "1–10K"},
		{bytes: 1024 * 1024, want: ">1M"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := classifySizeBandKey(tt.bytes); got != tt.want {
				t.Fatalf("classifySizeBandKey(%d) = %q, want %q", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestClassifyTokenBucket(t *testing.T) {
	tests := []struct {
		count int
		want  string
	}{
		{count: 16, want: " b16"},
		{count: 22, want: " b22"},
		{count: 18, want: " b18"},
		{count: 99, want: " other"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := classifyTokenBucket(tt.count); got != tt.want {
				t.Fatalf("classifyTokenBucket(%d) = %q, want %q", tt.count, got, tt.want)
			}
		})
	}
}
