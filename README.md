# llm-bench

> Continuous, open LLM API performance benchmark. Live at [bench.jonathanwrede.de](https://bench.jonathanwrede.de).

This repo contains the infrastructure that runs a 24/7 benchmark of major LLM
API endpoints and publishes the raw data as an open dataset.

Every hour, [llmprobe](https://github.com/Jwrede/llmprobe) sends a minimal
request to each tracked model and records time to first token (TTFT), total
latency, and generation throughput. Results are committed to the
[data repository](https://github.com/Jwrede/llm-bench-data) as JSONL.

A static dashboard at [bench.jonathanwrede.de](https://bench.jonathanwrede.de)
shows time series charts for all three metrics, regenerated after each probe
cycle.

## What it measures

| Metric | Description |
|--------|-------------|
| TTFT | Time from request to first content token (ms) |
| Latency | Total request duration (ms) |
| Tok/s | Tokens generated per second after first token |

## Model selection

Models are selected automatically based on OpenRouter weekly popularity
rankings. The `discover` command fetches the most actively used models and
picks the top 3 per provider. The model list refreshes daily.

Tracked providers: OpenAI, Anthropic, Google, DeepSeek, xAI.

All probes are routed through the OpenRouter API using a single OpenAI-compatible
endpoint, so the model identifiers include the provider prefix
(e.g. `anthropic/claude-sonnet-4.6`).

## Architecture

```
systemd timer (hourly)
  -> llmprobe probe (probes all models via OpenRouter, outputs JSON)
  -> append to results.jsonl
  -> generate (rebuild static site from accumulated data)

systemd timer (every :30)
  -> push-data.sh (commit results to data repo, clear local buffer)

systemd timer (daily at 00:30 UTC)
  -> discover --openrouter (query OpenRouter rankings, regenerate probes.yml)
```

The static site is served by Caddy with automatic HTTPS.

## Dataset

Raw JSONL data is published at
[github.com/Jwrede/llm-bench-data](https://github.com/Jwrede/llm-bench-data).

Each line is a JSON object:

```json
{
  "provider": "openai",
  "model": "anthropic/claude-sonnet-4.6",
  "status": "healthy",
  "ttft_ms": 312,
  "latency_ms": 2100,
  "tokens_per_sec": 68.4,
  "token_count": 20,
  "timestamp": "2026-05-06T14:00:00Z"
}
```

Files are organized by month and day: `data/2026-05/2026-05-06.jsonl`.

## Commands

### discover

Fetches OpenRouter weekly popularity rankings and generates a `probes.yml`
targeting the most used frontier models.

```bash
go build -o discover ./cmd/discover/
./discover --openrouter -max 3 -o probes.yml
```

Filtering: excludes free tiers, previews, betas, image/audio models, and
open-weight models (gemma).

### generate

Reads JSONL probe data and produces a static HTML dashboard with Chart.js
time series for TTFT, latency, and throughput.

```bash
go build -o generate ./cmd/generate/
./generate -data /opt/llm-bench/data -out /opt/llm-bench/site
```

## Deployment

Runs on a VPS with systemd timers. Caddy serves the static site.

```bash
# manual probe run
llmprobe probe -f json -c /opt/llm-bench/probes.yml | jq -c '.[]' >> data/results.jsonl

# manual discovery
/opt/llm-bench/discover --openrouter -max 3 -o /opt/llm-bench/probes.yml

# regenerate site
/opt/llm-bench/generate -data /opt/llm-bench/data -out /opt/llm-bench/site
```

## Known limitations

- **OpenRouter proxy overhead**: All probes currently route through OpenRouter
  rather than hitting provider APIs directly. This adds variable proxy latency,
  so TTFT and latency numbers are higher than what you would see calling each
  provider's native endpoint. Throughput (tok/s) is less affected. Switching to
  direct provider APIs is planned if the project gains traction.

## Cost

Probing 15 models hourly with "Hi" (1-2 input tokens, 20 output tokens max)
costs approximately $0.50-$1.50/month via OpenRouter.

## License

MIT
