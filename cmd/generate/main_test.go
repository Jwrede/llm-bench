package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMedian(t *testing.T) {
	tests := []struct {
		vals []float64
		want float64
	}{
		{nil, 0},
		{[]float64{5}, 5},
		{[]float64{1, 3, 5}, 3},
		{[]float64{1, 2, 3, 4}, 2.5},
		{[]float64{10, 1, 5, 3, 8}, 5},
	}

	for _, tt := range tests {
		got := median(tt.vals)
		if got != tt.want {
			t.Errorf("median(%v) = %v, want %v", tt.vals, got, tt.want)
		}
	}
}

func TestPercentile(t *testing.T) {
	vals := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	p50 := percentile(vals, 0.5)
	if p50 != 6 {
		t.Errorf("p50 = %v, want 6", p50)
	}

	p95 := percentile(vals, 0.95)
	if p95 != 10 {
		t.Errorf("p95 = %v, want 10", p95)
	}

	if percentile(nil, 0.5) != 0 {
		t.Error("percentile of nil should be 0")
	}
}

func TestParseTimestamp(t *testing.T) {
	ts := parseTimestamp("2026-05-06T14:00:00Z")
	if ts == 0 {
		t.Error("parseTimestamp returned 0 for valid timestamp")
	}

	ts2 := parseTimestamp("not-a-timestamp")
	if ts2 != 0 {
		t.Errorf("parseTimestamp returned %v for invalid timestamp, want 0", ts2)
	}
}

func TestDownsample(t *testing.T) {
	vals := make([]float64, 200)
	for i := range vals {
		vals[i] = float64(i)
	}

	ds := downsample(vals, 50)
	if len(ds) > 50 {
		t.Errorf("downsample produced %d items, want <= 50", len(ds))
	}

	short := []float64{1, 2, 3}
	ds2 := downsample(short, 50)
	if len(ds2) != 3 {
		t.Errorf("downsample should not reduce below max, got %d", len(ds2))
	}
}

func TestDownsamplePoints(t *testing.T) {
	pts := make([]DataPoint, 200)
	for i := range pts {
		pts[i] = DataPoint{T: float64(i * 1000), Y: float64(i)}
	}

	ds := downsamplePoints(pts, 50)
	if len(ds) > 50 {
		t.Errorf("downsamplePoints produced %d items, want <= 50", len(ds))
	}

	short := []DataPoint{{T: 1, Y: 1}, {T: 2, Y: 2}}
	ds2 := downsamplePoints(short, 50)
	if len(ds2) != 2 {
		t.Errorf("should not reduce below max, got %d", len(ds2))
	}
}

func TestLoadRecords(t *testing.T) {
	dir := t.TempDir()

	records := []Record{
		{Provider: "openai", Model: "openai/gpt-5.5", Status: "healthy", TTFTMs: 100, LatencyMs: 500, TokensPerSec: 50, TokenCount: 20, Timestamp: "2026-05-06T14:00:00Z"},
		{Provider: "openai", Model: "openai/gpt-5.5", Status: "healthy", TTFTMs: 120, LatencyMs: 600, TokensPerSec: 45, TokenCount: 20, Timestamp: "2026-05-06T15:00:00Z"},
	}

	f, err := os.Create(filepath.Join(dir, "results.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	enc := json.NewEncoder(f)
	for _, r := range records {
		enc.Encode(r)
	}
	f.Close()

	loaded := loadRecords(dir)
	if len(loaded) != 2 {
		t.Errorf("loaded %d records, want 2", len(loaded))
	}
}

func TestAggregate(t *testing.T) {
	records := []Record{
		{Provider: "openai", Model: "openai/gpt-5.5", Status: "healthy", TTFTMs: 100, LatencyMs: 500, TokensPerSec: 50, TokenCount: 20, Timestamp: "2026-05-06T14:00:00Z"},
		{Provider: "openai", Model: "openai/gpt-5.5", Status: "healthy", TTFTMs: 200, LatencyMs: 600, TokensPerSec: 40, TokenCount: 20, Timestamp: "2026-05-06T15:00:00Z"},
		{Provider: "openai", Model: "openai/gpt-5.5", Status: "error", Error: "timeout", Timestamp: "2026-05-06T16:00:00Z"},
		{Provider: "openai", Model: "anthropic/claude-sonnet-4.6", Status: "healthy", TTFTMs: 300, LatencyMs: 800, TokensPerSec: 60, TokenCount: 20, Timestamp: "2026-05-06T14:00:00Z"},
	}

	data := aggregate(records)

	if len(data.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(data.Models))
	}
	if data.TotalProbes != 3 {
		t.Errorf("expected 3 total probes, got %d", data.TotalProbes)
	}
	if data.Errors != 1 {
		t.Errorf("expected 1 error, got %d", data.Errors)
	}
}

func TestAggregateModelKey(t *testing.T) {
	records := []Record{
		{Provider: "openai", Model: "anthropic/claude-sonnet-4.6", Status: "healthy", TTFTMs: 100, LatencyMs: 500, TokensPerSec: 50, TokenCount: 20, Timestamp: "2026-05-06T14:00:00Z"},
		{Provider: "anthropic", Model: "claude-sonnet-4.6", Status: "healthy", TTFTMs: 100, LatencyMs: 500, TokensPerSec: 50, TokenCount: 20, Timestamp: "2026-05-06T14:00:00Z"},
	}

	data := aggregate(records)

	keys := make(map[string]bool)
	for _, m := range data.Models {
		keys[m.Key] = true
	}

	if !keys["anthropic/claude-sonnet-4.6"] {
		t.Error("expected model key 'anthropic/claude-sonnet-4.6'")
	}
}

func TestWriteHTML(t *testing.T) {
	dir := t.TempDir()

	data := TemplateData{
		Models: []ModelStats{
			{
				Key:     "openai/gpt-5.5",
				Color:   "#ef4444",
				TTFTP50: 100,
				TTFTP95: 200,
				LatP50:  500,
				TpsP50:  50,
				Probes:  5,
				TTFTPoints:    []DataPoint{{T: 1000, Y: 100}},
				LatencyPoints: []DataPoint{{T: 1000, Y: 500}},
				TpsPoints:     []DataPoint{{T: 1000, Y: 50}},
			},
		},
		TotalProbes: 5,
		Healthy:     5,
		Errors:      0,
		LastUpdate:  "2026-05-06T14:00:00Z",
	}

	err := writeHTML(dir, data)
	if err != nil {
		t.Fatalf("writeHTML failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "index.html"))
	if err != nil {
		t.Fatalf("could not read index.html: %v", err)
	}

	html := string(content)
	if len(html) < 100 {
		t.Error("generated HTML is suspiciously short")
	}
}
