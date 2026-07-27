// Copyright 2026 Jasen Minton
//
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"
)

type changeGoldenCorpus struct {
	Cases []changeGoldenCase `json:"cases"`
}

type changeGoldenCase struct {
	Name      string            `json:"name"`
	Reference changeGoldenInput `json:"reference"`
	Selected  changeGoldenInput `json:"selected"`
	Expected  changeGoldenWant  `json:"expected"`
}

type changeGoldenWant struct {
	Score      float64                 `json:"score"`
	Primary    string                  `json:"primary"`
	Components []changeGoldenComponent `json:"components"`
}

type changeGoldenComponent struct {
	Key   string  `json:"key"`
	Score float64 `json:"score"`
}

type changeGoldenInput struct {
	Lines     int                 `json:"lines"`
	Bytes     int                 `json:"bytes"`
	Errors    int                 `json:"errors"`
	Marked    int                 `json:"marked"`
	B16       int                 `json:"b16"`
	WordPeaks []changeGoldenEntry `json:"word_peaks"`
	RefPeaks  []changeGoldenEntry `json:"ref_peaks"`
	IPPeaks   []changeGoldenEntry `json:"ip_peaks"`
	WordWave  []changeGoldenEntry `json:"word_wave"`
}

type changeGoldenEntry struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

func TestChangeGoldenCorpus(t *testing.T) {
	data, err := os.ReadFile("testdata/change_golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus changeGoldenCorpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		t.Fatal(err)
	}

	for _, test := range corpus.Cases {
		t.Run(test.Name, func(t *testing.T) {
			result := compareChangeShapes(
				goldenChangeShape(test.Reference),
				goldenChangeShape(test.Selected),
			)
			if math.Abs(result.score-test.Expected.Score) > 1e-9 {
				t.Fatalf("score = %.15f, want %.15f", result.score, test.Expected.Score)
			}
			if result.components[0].key != test.Expected.Primary {
				t.Fatalf("primary = %q, want %q", result.components[0].key, test.Expected.Primary)
			}
			gotKeys := make([]string, len(result.components))
			for i, component := range result.components {
				gotKeys[i] = component.key
			}
			wantKeys := make([]string, len(test.Expected.Components))
			for i, component := range test.Expected.Components {
				wantKeys[i] = component.Key
			}
			if !reflect.DeepEqual(gotKeys, wantKeys) {
				t.Fatalf("component order = %v, want %v", gotKeys, wantKeys)
			}
			for i, component := range result.components {
				if math.Abs(component.score-test.Expected.Components[i].Score) > 1e-9 {
					t.Fatalf("component %q = %.15f, want %.15f",
						component.key, component.score, test.Expected.Components[i].Score)
				}
			}
		})
	}
}

func goldenChangeShape(input changeGoldenInput) changeShape {
	shape := changeShape{
		lines:     float64(input.Lines),
		bytes:     float64(input.Bytes),
		errors:    float64(input.Errors),
		wordPeaks: goldenDistribution(input.WordPeaks),
		refPeaks:  goldenDistribution(input.RefPeaks),
		ipPeaks:   goldenDistribution(input.IPPeaks),
		wordWave:  goldenDistribution(input.WordWave),
	}
	if input.Lines > 0 {
		shape.averageBytes = changeOptionalValue{value: float64(input.Bytes) / float64(input.Lines), available: true}
		shape.marked = changeOptionalValue{value: float64(input.Marked) * 100 / float64(input.Lines), available: true}
		shape.b16 = changeOptionalValue{value: float64(input.B16) * 100 / float64(input.Lines), available: true}
	}
	return shape
}

func goldenDistribution(entries []changeGoldenEntry) []changeDistributionEntry {
	result := make([]changeDistributionEntry, len(entries))
	for i, entry := range entries {
		result[i] = changeDistributionEntry{key: entry.Key, count: entry.Count}
	}
	return result
}
