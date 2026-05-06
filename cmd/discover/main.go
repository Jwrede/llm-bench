package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type RankingsResponse struct {
	Data struct {
		Models []RankedModel `json:"models"`
	} `json:"data"`
}

type RankedModel struct {
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
}

type ProviderConfig struct {
	Name    string        `yaml:"name"`
	APIKey  string        `yaml:"api_key,omitempty"`
	BaseURL string        `yaml:"base_url,omitempty"`
	Models  []ModelConfig `yaml:"models"`
}

type ModelConfig struct {
	Name string `yaml:"name"`
}

type ProbesConfig struct {
	Defaults  DefaultsConfig   `yaml:"defaults"`
	Providers []ProviderConfig `yaml:"providers"`
}

type DefaultsConfig struct {
	Prompt    string `yaml:"prompt"`
	MaxTokens int    `yaml:"max_tokens"`
	Timeout   string `yaml:"timeout"`
}

var targetProviders = map[string]bool{
	"openai":    true,
	"anthropic": true,
	"google":    true,
	"deepseek":  true,
	"x-ai":     true,
}

func main() {
	outPath := flag.String("o", "probes.yml", "output path for generated probes.yml")
	maxPerProvider := flag.Int("max", 3, "max models per provider")
	openrouter := flag.Bool("openrouter", false, "generate config for probing via OpenRouter instead of direct APIs")
	flag.Parse()

	models, err := fetchWeeklyRankings()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching rankings: %v\n", err)
		os.Exit(1)
	}

	filtered := filterByProvider(models, *maxPerProvider)

	var config ProbesConfig
	if *openrouter {
		config = buildOpenRouterConfig(filtered)
	} else {
		config = buildDirectConfig(filtered)
	}

	data, err := yaml.Marshal(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling yaml: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", *outPath, err)
		os.Exit(1)
	}

	total := 0
	for _, models := range filtered {
		total += len(models)
	}
	fmt.Printf("wrote %s (%d models from weekly rankings)\n", *outPath, total)
	for provider, models := range filtered {
		for _, m := range models {
			fmt.Printf("  %s: %s\n", provider, m.Slug)
		}
	}
}

func fetchWeeklyRankings() ([]RankedModel, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get("https://openrouter.ai/api/frontend/models/find?order=top-weekly")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result RankingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data.Models, nil
}

func filterByProvider(models []RankedModel, maxPerProvider int) map[string][]RankedModel {
	grouped := make(map[string][]RankedModel)

	for _, m := range models {
		provider := extractProvider(m.Slug)
		if !targetProviders[provider] {
			continue
		}
		if shouldExclude(m.Slug) {
			continue
		}
		if len(grouped[provider]) >= maxPerProvider {
			continue
		}
		grouped[provider] = append(grouped[provider], m)
	}

	return grouped
}

func extractProvider(slug string) string {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

func shouldExclude(slug string) bool {
	lower := strings.ToLower(slug)
	patterns := []string{":free", "preview", "beta", "image", "audio", "search", "gemma"}
	for _, p := range patterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func modelName(slug string) string {
	parts := strings.SplitN(slug, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return slug
}

func buildOpenRouterConfig(grouped map[string][]RankedModel) ProbesConfig {
	config := ProbesConfig{
		Defaults: DefaultsConfig{
			Prompt:    "Hi",
			MaxTokens: 20,
			Timeout:   "30s",
		},
	}

	entry := ProviderConfig{
		Name:    "openai",
		APIKey:  "${OPENROUTER_API_KEY}",
		BaseURL: "https://openrouter.ai/api",
	}

	providerOrder := []string{"openai", "anthropic", "google", "deepseek", "x-ai"}
	for _, provider := range providerOrder {
		models, ok := grouped[provider]
		if !ok {
			continue
		}
		for _, m := range models {
			entry.Models = append(entry.Models, ModelConfig{Name: m.Slug})
		}
	}

	config.Providers = append(config.Providers, entry)
	return config
}

func buildDirectConfig(grouped map[string][]RankedModel) ProbesConfig {
	config := ProbesConfig{
		Defaults: DefaultsConfig{
			Prompt:    "Hi",
			MaxTokens: 20,
			Timeout:   "30s",
		},
	}

	type providerInfo struct {
		configName string
		apiKeyEnv  string
		baseURL    string
	}

	providerMap := map[string]providerInfo{
		"openai":    {configName: "openai", apiKeyEnv: "OPENAI_API_KEY"},
		"anthropic": {configName: "anthropic", apiKeyEnv: "ANTHROPIC_API_KEY"},
		"google":    {configName: "google", apiKeyEnv: "GEMINI_API_KEY"},
		"deepseek":  {configName: "openai", apiKeyEnv: "DEEPSEEK_API_KEY", baseURL: "https://api.deepseek.com/v1"},
		"x-ai":     {configName: "openai", apiKeyEnv: "XAI_API_KEY", baseURL: "https://api.x.ai/v1"},
	}

	providerOrder := []string{"openai", "anthropic", "google", "deepseek", "x-ai"}
	for _, provider := range providerOrder {
		models, ok := grouped[provider]
		if !ok {
			continue
		}
		info := providerMap[provider]
		entry := ProviderConfig{
			Name:    info.configName,
			APIKey:  fmt.Sprintf("${%s}", info.apiKeyEnv),
			BaseURL: info.baseURL,
		}
		for _, m := range models {
			entry.Models = append(entry.Models, ModelConfig{Name: modelName(m.Slug)})
		}
		config.Providers = append(config.Providers, entry)
	}

	return config
}
