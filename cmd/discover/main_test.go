package main

import (
	"testing"
)

func TestExtractProvider(t *testing.T) {
	tests := []struct {
		slug string
		want string
	}{
		{"openai/gpt-5.5", "openai"},
		{"anthropic/claude-sonnet-4.6", "anthropic"},
		{"google/gemini-2.5-flash", "google"},
		{"deepseek/deepseek-v4-pro", "deepseek"},
		{"x-ai/grok-4.3", "x-ai"},
		{"no-slash", ""},
	}

	for _, tt := range tests {
		got := extractProvider(tt.slug)
		if got != tt.want {
			t.Errorf("extractProvider(%q) = %q, want %q", tt.slug, got, tt.want)
		}
	}
}

func TestShouldExclude(t *testing.T) {
	excluded := []string{
		"openai/gpt-4o:free",
		"anthropic/claude-3-preview",
		"google/gemma-7b",
		"openai/gpt-4-beta",
		"openai/dall-e-image-gen",
		"openai/whisper-audio",
		"google/gemini-search",
	}
	for _, slug := range excluded {
		if !shouldExclude(slug) {
			t.Errorf("shouldExclude(%q) = false, want true", slug)
		}
	}

	allowed := []string{
		"openai/gpt-5.5",
		"anthropic/claude-sonnet-4.6",
		"deepseek/deepseek-v4-pro",
		"x-ai/grok-4.3",
	}
	for _, slug := range allowed {
		if shouldExclude(slug) {
			t.Errorf("shouldExclude(%q) = true, want false", slug)
		}
	}
}

func TestModelName(t *testing.T) {
	tests := []struct {
		slug string
		want string
	}{
		{"openai/gpt-5.5", "gpt-5.5"},
		{"anthropic/claude-sonnet-4.6", "claude-sonnet-4.6"},
		{"no-slash", "no-slash"},
	}

	for _, tt := range tests {
		got := modelName(tt.slug)
		if got != tt.want {
			t.Errorf("modelName(%q) = %q, want %q", tt.slug, got, tt.want)
		}
	}
}

func TestFilterByProvider(t *testing.T) {
	models := []RankedModel{
		{Slug: "openai/gpt-5.5"},
		{Slug: "openai/gpt-5.4"},
		{Slug: "openai/gpt-oss-120b"},
		{Slug: "openai/gpt-4o-mini"},
		{Slug: "anthropic/claude-sonnet-4.6"},
		{Slug: "anthropic/claude-opus-4.7"},
		{Slug: "google/gemini-2.5-flash"},
		{Slug: "mistralai/mistral-large"},
		{Slug: "openai/gpt-4o:free"},
	}

	grouped := filterByProvider(models, 2)

	if len(grouped["openai"]) != 2 {
		t.Errorf("expected 2 openai models, got %d", len(grouped["openai"]))
	}
	if len(grouped["anthropic"]) != 2 {
		t.Errorf("expected 2 anthropic models, got %d", len(grouped["anthropic"]))
	}
	if len(grouped["google"]) != 1 {
		t.Errorf("expected 1 google model, got %d", len(grouped["google"]))
	}
	if _, ok := grouped["mistralai"]; ok {
		t.Error("mistralai should not be in target providers")
	}
}

func TestBuildOpenRouterConfig(t *testing.T) {
	grouped := map[string][]RankedModel{
		"openai":    {{Slug: "openai/gpt-5.5"}},
		"anthropic": {{Slug: "anthropic/claude-sonnet-4.6"}},
	}

	config := buildOpenRouterConfig(grouped)

	if len(config.Providers) != 1 {
		t.Fatalf("expected 1 provider entry, got %d", len(config.Providers))
	}
	if config.Providers[0].Name != "openai" {
		t.Errorf("expected provider name 'openai', got %q", config.Providers[0].Name)
	}
	if len(config.Providers[0].Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(config.Providers[0].Models))
	}
	if config.Providers[0].BaseURL != "https://openrouter.ai/api" {
		t.Errorf("expected OpenRouter base URL, got %q", config.Providers[0].BaseURL)
	}
}

func TestBuildDirectConfig(t *testing.T) {
	grouped := map[string][]RankedModel{
		"openai":   {{Slug: "openai/gpt-5.5"}},
		"deepseek": {{Slug: "deepseek/deepseek-v4-pro"}},
	}

	config := buildDirectConfig(grouped)

	if len(config.Providers) != 2 {
		t.Fatalf("expected 2 provider entries, got %d", len(config.Providers))
	}

	for _, p := range config.Providers {
		if p.APIKey == "" {
			t.Errorf("provider %q has empty API key", p.Name)
		}
	}
}
