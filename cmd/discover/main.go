package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type OpenRouterModel struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Created       int64           `json:"created"`
	ContextLength int             `json:"context_length"`
	Pricing       OpenRouterPrice `json:"pricing"`
}

type OpenRouterPrice struct {
	Prompt     string `json:"prompt"`
	Completion string `json:"completion"`
}

type OpenRouterResponse struct {
	Data []OpenRouterModel `json:"data"`
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

var providers = map[string]struct {
	configName string
	apiKeyEnv  string
	baseURL    string
}{
	"openai":    {configName: "openai", apiKeyEnv: "OPENAI_API_KEY", baseURL: ""},
	"anthropic": {configName: "anthropic", apiKeyEnv: "ANTHROPIC_API_KEY", baseURL: ""},
	"google":    {configName: "google", apiKeyEnv: "GEMINI_API_KEY", baseURL: ""},
	"deepseek":  {configName: "openai", apiKeyEnv: "DEEPSEEK_API_KEY", baseURL: "https://api.deepseek.com/v1"},
	"x-ai":     {configName: "openai", apiKeyEnv: "XAI_API_KEY", baseURL: "https://api.x.ai/v1"},
}

func main() {
	outPath := flag.String("o", "probes.yml", "output path for generated probes.yml")
	maxPerProvider := flag.Int("max", 3, "max models per provider")
	minContext := flag.Int("min-context", 32768, "minimum context length")
	flag.Parse()

	models, err := fetchOpenRouterModels()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error fetching models: %v\n", err)
		os.Exit(1)
	}

	filtered := filterFrontierModels(models, *maxPerProvider, *minContext)
	config := buildProbesConfig(filtered)

	data, err := yaml.Marshal(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error marshaling yaml: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outPath, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", *outPath, err)
		os.Exit(1)
	}

	fmt.Printf("wrote %s (%d models across %d provider entries)\n", *outPath, countModels(config), len(config.Providers))
}

func fetchOpenRouterModels() ([]OpenRouterModel, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get("https://openrouter.ai/api/v1/models")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result OpenRouterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Data, nil
}

func filterFrontierModels(models []OpenRouterModel, maxPerProvider, minContext int) map[string][]OpenRouterModel {
	grouped := make(map[string][]OpenRouterModel)

	for _, m := range models {
		provider := extractProvider(m.ID)
		if provider == "" {
			continue
		}
		if m.ContextLength < minContext {
			continue
		}
		if m.Pricing.Completion == "0" || m.Pricing.Completion == "" {
			continue
		}
		if shouldExclude(m.ID) {
			continue
		}
		grouped[provider] = append(grouped[provider], m)
	}

	for provider, list := range grouped {
		sort.Slice(list, func(i, j int) bool {
			return list[i].Created > list[j].Created
		})
		if len(list) > maxPerProvider {
			list = list[:maxPerProvider]
		}
		grouped[provider] = list
	}

	return grouped
}

func extractProvider(id string) string {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return ""
	}
	prefix := parts[0]
	if _, ok := providers[prefix]; ok {
		return prefix
	}
	return ""
}

func shouldExclude(id string) bool {
	lower := strings.ToLower(id)
	excludePatterns := []string{":free", ":extended", "preview", "beta", "image", "multi-agent", "latest"}
	for _, p := range excludePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	if !matchesProviderPattern(id) {
		return true
	}
	return false
}

func matchesProviderPattern(id string) bool {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) != 2 {
		return false
	}
	provider, model := parts[0], parts[1]
	switch provider {
	case "openai":
		return strings.HasPrefix(model, "gpt-") || strings.HasPrefix(model, "o")
	case "anthropic":
		return strings.HasPrefix(model, "claude-")
	case "google":
		return strings.HasPrefix(model, "gemini-")
	case "deepseek":
		return strings.HasPrefix(model, "deepseek-")
	case "x-ai":
		return strings.HasPrefix(model, "grok-")
	}
	return true
}

func buildProbesConfig(grouped map[string][]OpenRouterModel) ProbesConfig {
	config := ProbesConfig{
		Defaults: DefaultsConfig{
			Prompt:    "Hi",
			MaxTokens: 20,
			Timeout:   "30s",
		},
	}

	providerEntries := make(map[string]*ProviderConfig)

	providerOrder := []string{"openai", "anthropic", "google", "deepseek", "x-ai"}
	for _, providerName := range providerOrder {
		models, ok := grouped[providerName]
		if !ok {
			continue
		}

		info := providers[providerName]
		key := fmt.Sprintf("%s_%s", info.configName, info.baseURL)

		entry, exists := providerEntries[key]
		if !exists {
			entry = &ProviderConfig{
				Name:    info.configName,
				APIKey:  fmt.Sprintf("${%s}", info.apiKeyEnv),
				BaseURL: info.baseURL,
			}
			providerEntries[key] = entry
			config.Providers = append(config.Providers, *entry)
		}

		for _, m := range models {
			modelName := extractModelName(m.ID)
			entry.Models = append(entry.Models, ModelConfig{Name: modelName})
		}

		for i := range config.Providers {
			if config.Providers[i].Name == entry.Name && config.Providers[i].BaseURL == entry.BaseURL {
				config.Providers[i] = *entry
			}
		}
	}

	return config
}

func extractModelName(openRouterID string) string {
	parts := strings.SplitN(openRouterID, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return openRouterID
}

func countModels(config ProbesConfig) int {
	count := 0
	for _, p := range config.Providers {
		count += len(p.Models)
	}
	return count
}
