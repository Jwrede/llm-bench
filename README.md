# llm-bench

> Continuous, open LLM API performance benchmark. Live TUI at [bench.jonathanwrede.de](https://bench.jonathanwrede.de).

This repo contains the infrastructure that runs a 24/7 benchmark of major LLM
API endpoints and publishes the raw data as an open dataset.

Every hour, [llmprobe](https://github.com/Jwrede/llmprobe) sends a minimal
request to each tracked model and records time to first token (TTFT), total
latency, and generation throughput. The results are committed to the
[data repository](https://github.com/Jwrede/llm-bench-data) as JSONL.

A live terminal dashboard is served at
[bench.jonathanwrede.de](https://bench.jonathanwrede.de) via
[ttyd](https://github.com/nicm/ttyd).

## What it measures

| Metric | Description |
|--------|-------------|
| TTFT | Time from request to first content token (ms) |
| Latency | Total request duration (ms) |
| Tok/s | Tokens generated per second after first token |

## Tracked providers

Models are automatically discovered via the
[OpenRouter API](https://openrouter.ai/api/v1/models) and probed against
their native endpoints (not OpenRouter) for accurate latency measurement.

| Provider | Endpoint |
|----------|----------|
| OpenAI | api.openai.com |
| Anthropic | api.anthropic.com |
| Google | generativelanguage.googleapis.com |
| DeepSeek | api.deepseek.com |
| xAI | api.x.ai |

The model list updates daily. New frontier models are added automatically;
deprecated models are removed.

## Architecture

```
systemd timer (hourly)
  -> llmprobe watch -f json (probes all models, outputs JSONL)
  -> push-data.sh (commits results to data repo)

systemd timer (daily)
  -> discover (queries OpenRouter, regenerates probes.yml)
  -> restarts probe services with updated model list

ttyd (persistent)
  -> llmprobe watch --tui (live terminal dashboard)
  -> nginx reverse proxy at bench.jonathanwrede.de
```

## Dataset

Raw JSONL data is published at
[github.com/Jwrede/llm-bench-data](https://github.com/Jwrede/llm-bench-data).

Each line is a JSON object:

```json
{
  "provider": "openai",
  "model": "gpt-4o",
  "ttft_ms": 312,
  "latency_ms": 2100,
  "tokens_per_sec": 68.4,
  "token_count": 20,
  "status": "ok",
  "timestamp": "2026-05-06T14:00:00Z"
}
```

Files are organized by month and day: `data/2026-05/2026-05-06.jsonl`.

## Discovery tool

The `discover` command fetches the OpenRouter model catalog and generates a
`probes.yml` configuration targeting frontier models from each provider.

```bash
go build -o discover ./cmd/discover/
./discover -o probes.yml -max 3 -min-context 32768
```

Filtering criteria:
- Top 3 most recent models per provider
- Minimum 32k context window
- Excludes free tiers, previews, and beta models
- Only models with non-zero completion pricing

## Deployment

Runs on a VPS with systemd. See `deploy/` for service files and nginx config.

```bash
# initial setup
./scripts/setup.sh git@github.com:Jwrede/llm-bench-data.git

# manual probe run
llmprobe probe -f json -c /opt/llm-bench/probes.yml

# manual discovery
/opt/llm-bench/discover -o /opt/llm-bench/probes.yml
```

## Cost

Probing 13 models hourly with "Hi" (1-2 input tokens, 20 output tokens max)
costs approximately $0.50-$1.50/month across all providers.

## License

MIT
