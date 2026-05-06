package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Record struct {
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Status       string  `json:"status"`
	TTFTMs       float64 `json:"ttft_ms"`
	LatencyMs    float64 `json:"latency_ms"`
	TokensPerSec float64 `json:"tokens_per_sec"`
	TokenCount   int     `json:"token_count"`
	Error        string  `json:"error"`
	Timestamp    string  `json:"timestamp"`
}

type DataPoint struct {
	T float64 `json:"x"`
	Y float64 `json:"y"`
}

type ModelStats struct {
	Key         string
	Color       string
	TTFTP50     float64
	TTFTP95     float64
	LatP50      float64
	TpsP50      float64
	Errors      int
	Probes      int
	TTFTData    []float64
	LatencyData []float64
	TpsData     []float64
	TTFTPoints    []DataPoint
	LatencyPoints []DataPoint
	TpsPoints     []DataPoint
}

type TemplateData struct {
	Models     []ModelStats
	TotalProbes int
	Healthy    int
	Errors     int
	LastUpdate string
}

var colors = []string{
	"#ef4444", // red
	"#22c55e", // green
	"#3b82f6", // blue
	"#eab308", // yellow
	"#a855f7", // magenta
	"#06b6d4", // cyan
	"#f8fafc", // white
	"#f97316", // orange
}

func main() {
	dataDir := flag.String("data", "/opt/llm-bench/data", "path to data directory")
	outDir := flag.String("out", "/opt/llm-bench/site", "output directory for generated site")
	flag.Parse()

	records := loadRecords(*dataDir)
	data := aggregate(records)

	if err := os.MkdirAll(*outDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating output dir: %v\n", err)
		os.Exit(1)
	}

	if err := writeHTML(*outDir, data); err != nil {
		fmt.Fprintf(os.Stderr, "error writing html: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("generated site at %s (%d models, %d probes)\n", *outDir, len(data.Models), data.TotalProbes)
}

func loadRecords(dataDir string) []Record {
	var records []Record

	filepath.Walk(dataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 || line[0] != '{' {
				continue
			}
			var rec Record
			if json.Unmarshal(line, &rec) == nil {
				records = append(records, rec)
			}
		}
		return nil
	})

	file := filepath.Join(dataDir, "results.jsonl")
	if f, err := os.Open(file); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Bytes()
			if len(line) == 0 || line[0] != '{' {
				continue
			}
			var rec Record
			if json.Unmarshal(line, &rec) == nil {
				records = append(records, rec)
			}
		}
	}

	return records
}

func aggregate(records []Record) TemplateData {
	byModel := make(map[string]*ModelStats)
	var order []string

	for _, r := range records {
		key := r.Model
		if !strings.Contains(key, "/") {
			key = r.Provider + "/" + key
		}
		ms, ok := byModel[key]
		if !ok {
			ms = &ModelStats{Key: key}
			byModel[key] = ms
			order = append(order, key)
		}

		if r.Status == "error" {
			ms.Errors++
			continue
		}

		ms.TTFTData = append(ms.TTFTData, r.TTFTMs)
		ms.LatencyData = append(ms.LatencyData, r.LatencyMs)
		ms.TpsData = append(ms.TpsData, r.TokensPerSec)

		ts := parseTimestamp(r.Timestamp)
		ms.TTFTPoints = append(ms.TTFTPoints, DataPoint{T: ts, Y: r.TTFTMs})
		ms.LatencyPoints = append(ms.LatencyPoints, DataPoint{T: ts, Y: r.LatencyMs})
		ms.TpsPoints = append(ms.TpsPoints, DataPoint{T: ts, Y: r.TokensPerSec})
		ms.Probes++
	}

	sort.Strings(order)

	var models []ModelStats
	totalProbes := 0
	totalErrors := 0

	for i, key := range order {
		ms := byModel[key]
		ms.Color = colors[i%len(colors)]

		if ms.Probes > 0 {
			ms.TTFTP50 = median(ms.TTFTData)
			ms.TTFTP95 = percentile(ms.TTFTData, 0.95)
			ttftForLat := make([]float64, len(ms.TTFTData))
			copy(ttftForLat, ms.TTFTData)
			ms.LatP50 = ms.TTFTP50
			ms.TpsP50 = computeMedianTps(records, key)
			ms.LatP50 = computeMedianLat(records, key)
		}

		totalProbes += ms.Probes
		totalErrors += ms.Errors
		models = append(models, *ms)
	}

	lastUpdate := ""
	if len(records) > 0 {
		lastUpdate = records[len(records)-1].Timestamp
	}

	return TemplateData{
		Models:      models,
		TotalProbes: totalProbes,
		Healthy:     totalProbes,
		Errors:      totalErrors,
		LastUpdate:  lastUpdate,
	}
}

func recordKey(r Record) string {
	if strings.Contains(r.Model, "/") {
		return r.Model
	}
	return r.Provider + "/" + r.Model
}

func computeMedianTps(records []Record, key string) float64 {
	var vals []float64
	for _, r := range records {
		if recordKey(r) == key && r.Status != "error" {
			vals = append(vals, r.TokensPerSec)
		}
	}
	return median(vals)
}

func computeMedianLat(records []Record, key string) float64 {
	var vals []float64
	for _, r := range records {
		if recordKey(r) == key && r.Status != "error" {
			vals = append(vals, r.LatencyMs)
		}
	}
	return median(vals)
}

func median(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	sort.Float64s(sorted)
	idx := int(math.Floor(float64(len(sorted)) * p))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func parseTimestamp(ts string) float64 {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05Z", ts)
		if err != nil {
			return 0
		}
	}
	return float64(t.UnixMilli())
}

func downsamplePoints(pts []DataPoint, max int) []DataPoint {
	if len(pts) <= max {
		return pts
	}
	bucket := len(pts) / max
	out := make([]DataPoint, 0, max)
	for i := 0; i < len(pts); i += bucket {
		end := i + bucket
		if end > len(pts) {
			end = len(pts)
		}
		mid := (i + end) / 2
		out = append(out, pts[mid])
	}
	return out
}

func downsample(vals []float64, max int) []float64 {
	if len(vals) <= max {
		return vals
	}
	bucket := len(vals) / max
	out := make([]float64, 0, max)
	for i := 0; i < len(vals); i += bucket {
		end := i + bucket
		if end > len(vals) {
			end = len(vals)
		}
		chunk := vals[i:end]
		sorted := make([]float64, len(chunk))
		copy(sorted, chunk)
		sort.Float64s(sorted)
		out = append(out, sorted[len(sorted)/2])
	}
	return out
}

func writeHTML(outDir string, data TemplateData) error {
	tmpl, err := template.New("index").Funcs(template.FuncMap{
		"json": func(v interface{}) template.JS {
			b, _ := json.Marshal(v)
			return template.JS(b)
		},
		"downsample": func(vals []float64) []float64 {
			return downsample(vals, 120)
		},
		"points": func(pts []DataPoint) template.JS {
			ds := downsamplePoints(pts, 120)
			b, _ := json.Marshal(ds)
			return template.JS(b)
		},
	}).Parse(htmlTemplate)
	if err != nil {
		return err
	}

	f, err := os.Create(filepath.Join(outDir, "index.html"))
	if err != nil {
		return err
	}
	defer f.Close()

	return tmpl.Execute(f, data)
}

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>llm-bench</title>
<meta http-equiv="refresh" content="300">
<script src="https://cdn.jsdelivr.net/npm/chart.js@4"></script>
<script src="https://cdn.jsdelivr.net/npm/chartjs-adapter-date-fns@3"></script>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }

:root {
  --bg: #09090b;
  --surface: #111113;
  --surface-2: #18181b;
  --border: #27272a;
  --text: #fafafa;
  --text-muted: #a1a1aa;
  --text-dim: #71717a;
  --accent: #22c55e;
}

body {
  background: var(--bg);
  color: var(--text);
  font-family: -apple-system, BlinkMacSystemFont, 'Inter', 'Segoe UI', sans-serif;
  font-size: 18px;
  line-height: 1.6;
  min-height: 100vh;
  padding: 48px 64px;
}

.header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 48px;
}

.header h1 {
  font-size: 32px;
  font-weight: 700;
  letter-spacing: -0.03em;
}

.badge {
  display: inline-block;
  font-size: 14px;
  padding: 4px 14px;
  border-radius: 100px;
  background: rgba(34, 197, 94, 0.1);
  color: var(--accent);
  border: 1px solid rgba(34, 197, 94, 0.2);
  margin-left: 16px;
  vertical-align: middle;
}

.header-right {
  font-size: 15px;
  color: var(--text-dim);
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 16px;
  margin-bottom: 48px;
}

.stat-card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 16px;
  padding: 28px 32px;
}

.stat-value {
  font-size: 36px;
  font-weight: 700;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  letter-spacing: -0.02em;
}

.stat-label {
  font-size: 14px;
  color: var(--text-dim);
  text-transform: uppercase;
  letter-spacing: 0.08em;
  margin-top: 8px;
}

.section {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 16px;
  margin-bottom: 48px;
  overflow: hidden;
}

.section-header {
  padding: 24px 32px;
  border-bottom: 1px solid var(--border);
}

.section-header h2 {
  font-size: 16px;
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.chart-body {
  padding: 32px;
}

.chart-wrapper {
  height: 50vh;
  min-height: 400px;
  position: relative;
}

.chart-wrapper-half {
  height: 36vh;
  min-height: 280px;
  position: relative;
}

.charts-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 24px;
  margin-bottom: 48px;
}

.legend {
  display: flex;
  flex-wrap: wrap;
  gap: 16px 32px;
  padding: 20px 32px;
  border-top: 1px solid var(--border);
  background: var(--surface-2);
}

.legend-item {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 15px;
  color: var(--text-muted);
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
}

.legend-dot {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  flex-shrink: 0;
}

table {
  width: 100%;
  border-collapse: collapse;
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 16px;
}

thead th {
  text-align: left;
  padding: 18px 28px;
  color: var(--text-dim);
  font-weight: 500;
  font-size: 13px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  border-bottom: 1px solid var(--border);
  background: var(--surface);
}

thead th:not(:first-child) {
  text-align: right;
}

tbody td {
  padding: 18px 28px;
  border-bottom: 1px solid var(--border);
}

tbody td:not(:first-child) {
  text-align: right;
  color: var(--text-muted);
}

tbody tr {
  transition: background 0.15s;
}

tbody tr:hover {
  background: var(--surface-2);
}

tbody tr:last-child td {
  border-bottom: none;
}

.model-name {
  display: flex;
  align-items: center;
  gap: 12px;
  font-weight: 500;
}

.model-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  flex-shrink: 0;
}

.footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 15px;
  color: var(--text-dim);
  padding-top: 16px;
}

.footer a {
  color: var(--text-muted);
  text-decoration: none;
  transition: color 0.15s;
}

.footer a:hover {
  color: var(--text);
}

.no-data {
  text-align: center;
  padding: 120px 20px;
}

.no-data h2 {
  font-size: 24px;
  color: var(--text-muted);
}

.no-data p {
  margin-top: 12px;
  font-size: 16px;
  color: var(--text-dim);
}

@media (max-width: 1024px) {
  body { padding: 32px; }
  .stats-row { grid-template-columns: repeat(2, 1fr); }
  .chart-wrapper { height: 40vh; min-height: 300px; }
  .charts-row { grid-template-columns: 1fr; }
  .chart-wrapper-half { height: 36vh; min-height: 260px; }
}

@media (max-width: 640px) {
  body { padding: 20px; font-size: 16px; }
  .header { flex-direction: column; align-items: flex-start; gap: 12px; }
  .header h1 { font-size: 24px; }
  .stats-row { grid-template-columns: 1fr 1fr; gap: 12px; }
  .stat-card { padding: 20px; }
  .stat-value { font-size: 28px; }
  .chart-wrapper { height: 300px; min-height: unset; }
  .section-header { padding: 18px 20px; }
  .chart-body { padding: 20px; }
  .legend { padding: 16px 20px; gap: 12px 20px; }
  .legend-item { font-size: 13px; }
  thead th, tbody td { padding: 14px 16px; }
  table { font-size: 14px; }
}
</style>
</head>
<body>

<div class="header">
  <h1>llm-bench<span class="badge">live</span></h1>
  <div class="header-right">
    {{if .LastUpdate}}updated {{.LastUpdate}}{{end}}
  </div>
</div>

{{if .Models}}
<div class="stats-row">
  <div class="stat-card">
    <div class="stat-value">{{len .Models}}</div>
    <div class="stat-label">Models</div>
  </div>
  <div class="stat-card">
    <div class="stat-value">{{.TotalProbes}}</div>
    <div class="stat-label">Total Probes</div>
  </div>
  <div class="stat-card">
    <div class="stat-value">{{.Healthy}}</div>
    <div class="stat-label">Healthy</div>
  </div>
  <div class="stat-card">
    <div class="stat-value">{{.Errors}}</div>
    <div class="stat-label">Errors</div>
  </div>
</div>

<div class="section">
  <div class="section-header">
    <h2>Time to First Token</h2>
  </div>
  <div class="chart-body">
    <div class="chart-wrapper">
      <canvas id="ttftChart"></canvas>
    </div>
  </div>
</div>

<div class="charts-row">
  <div class="section">
    <div class="section-header">
      <h2>Total Latency</h2>
    </div>
    <div class="chart-body">
      <div class="chart-wrapper-half">
        <canvas id="latencyChart"></canvas>
      </div>
    </div>
  </div>

  <div class="section">
    <div class="section-header">
      <h2>Throughput (tok/s)</h2>
    </div>
    <div class="chart-body">
      <div class="chart-wrapper-half">
        <canvas id="tpsChart"></canvas>
      </div>
    </div>
  </div>
</div>

<div class="section">
  <div class="section-header">
    <h2>Statistics</h2>
  </div>
  <table>
    <thead>
      <tr>
        <th>Model</th>
        <th>TTFT p50</th>
        <th>TTFT p95</th>
        <th>Latency</th>
        <th>Tok/s</th>
        <th>Errors</th>
        <th>N</th>
      </tr>
    </thead>
    <tbody>
      {{range .Models}}
      <tr>
        <td>
          <div class="model-name">
            <div class="model-dot" style="background: {{.Color}}"></div>
            {{.Key}}
          </div>
        </td>
        {{if gt .Probes 0}}
        <td>{{printf "%.0f" .TTFTP50}}ms</td>
        <td>{{printf "%.0f" .TTFTP95}}ms</td>
        <td>{{printf "%.0f" .LatP50}}ms</td>
        <td>{{printf "%.1f" .TpsP50}}</td>
        {{else}}
        <td>-</td><td>-</td><td>-</td><td>-</td>
        {{end}}
        <td>{{.Errors}}</td>
        <td>{{.Probes}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
</div>
{{else}}
<div class="no-data">
  <h2>Waiting for data</h2>
  <p>First probe results will appear here shortly.</p>
</div>
{{end}}

<div class="footer">
  <span>Probing every hour via <a href="https://github.com/Jwrede/llmprobe">llmprobe</a></span>
  <span><a href="https://github.com/Jwrede/llm-bench">source</a> &middot; <a href="https://github.com/Jwrede/llm-bench-data">dataset</a></span>
</div>

{{if .Models}}
<script>
const chartOpts = (unit, suffix) => ({
  responsive: true,
  maintainAspectRatio: false,
  animation: { duration: 0 },
  interaction: { intersect: false, mode: 'index' },
  plugins: {
    legend: {
      display: true,
      position: 'bottom',
      labels: {
        color: '#a1a1aa',
        font: { family: 'JetBrains Mono, monospace', size: 14 },
        padding: 24,
        usePointStyle: true,
        pointStyle: 'line',
        pointStyleWidth: 24,
      },
    },
    tooltip: {
      backgroundColor: '#18181b',
      titleColor: '#fafafa',
      bodyColor: '#a1a1aa',
      borderColor: '#3f3f46',
      borderWidth: 1,
      padding: 14,
      cornerRadius: 8,
      titleFont: { family: 'JetBrains Mono, monospace', size: 13 },
      bodyFont: { family: 'JetBrains Mono, monospace', size: 14 },
      callbacks: {
        label: function(ctx) {
          return ' ' + ctx.dataset.label + '  ' + Math.round(ctx.parsed.y) + suffix;
        }
      }
    },
  },
  scales: {
    x: {
      type: 'time',
      time: { unit: unit, displayFormats: { second: 'HH:mm:ss', minute: 'HH:mm', hour: 'MMM d HH:mm', day: 'MMM d' } },
      grid: { color: '#1a1a1e' },
      border: { display: false },
      ticks: { color: '#71717a', font: { family: 'JetBrains Mono, monospace', size: 12 }, maxTicksLimit: 8 },
    },
    y: {
      grid: { color: '#27272a', lineWidth: 0.5 },
      border: { display: false },
      ticks: {
        color: '#71717a',
        font: { family: 'JetBrains Mono, monospace', size: 13 },
        callback: function(v) { return v + suffix; },
        maxTicksLimit: 8,
        padding: 12,
      },
    },
  },
});

const lineStyle = (color) => ({
  borderColor: color,
  backgroundColor: color + '15',
  borderWidth: 2.5,
  pointRadius: 0,
  pointHoverRadius: 5,
  tension: 0.3,
  fill: true,
});

// TTFT chart
new Chart(document.getElementById('ttftChart').getContext('2d'), {
  type: 'line',
  data: {
    datasets: [
{{range .Models}}{{if gt .Probes 0}}
      { label: '{{.Key}}', data: {{points .TTFTPoints}}, ...lineStyle('{{.Color}}') },
{{end}}{{end}}
    ],
  },
  options: chartOpts('minute', 'ms'),
});

// Latency chart
new Chart(document.getElementById('latencyChart').getContext('2d'), {
  type: 'line',
  data: {
    datasets: [
{{range .Models}}{{if gt .Probes 0}}
      { label: '{{.Key}}', data: {{points .LatencyPoints}}, ...lineStyle('{{.Color}}') },
{{end}}{{end}}
    ],
  },
  options: chartOpts('minute', 'ms'),
});

// Throughput chart
new Chart(document.getElementById('tpsChart').getContext('2d'), {
  type: 'line',
  data: {
    datasets: [
{{range .Models}}{{if gt .Probes 0}}
      { label: '{{.Key}}', data: {{points .TpsPoints}}, ...lineStyle('{{.Color}}') },
{{end}}{{end}}
    ],
  },
  options: chartOpts('minute', ' t/s'),
});
</script>
{{end}}
</body>
</html>
`
