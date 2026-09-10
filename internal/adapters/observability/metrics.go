package observability

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

var _ ports.MetricsRecorder = (*PrometheusMetricsRecorder)(nil)

var defaultBuckets = []float64{0.1, 0.5, 1.0, 2.5, 5.0, 10.0, 20.0, 30.0, 45.0, 60.0}

// PrometheusMetricsRecorder implementa a captura de métricas do Prometheus para a API.
type PrometheusMetricsRecorder struct {
	mu sync.RWMutex

	// Contadores
	conversionsTotal  map[string]int64 // key: sourceFormat|targetFormat|status
	edgeRequestsTotal map[string]int64 // key: method|endpoint|statusCode|errorCode

	// Gauges
	queueDepth map[string]int64 // key: queueName

	// Histogramas com Buckets Explícitos
	conversionDurationSum     map[string]float64
	conversionDurationCount   map[string]int64
	conversionDurationBuckets map[string]map[float64]int64 // key: srcFormat|targetFormat -> bucketLe -> count

	dwellTimeSum     map[string]float64
	dwellTimeCount   map[string]int64
	dwellTimeBuckets map[string]map[float64]int64
}

func NewPrometheusMetricsRecorder() *PrometheusMetricsRecorder {
	return &PrometheusMetricsRecorder{
		conversionsTotal:          make(map[string]int64),
		edgeRequestsTotal:         make(map[string]int64),
		queueDepth:                make(map[string]int64),
		conversionDurationSum:     make(map[string]float64),
		conversionDurationCount:   make(map[string]int64),
		conversionDurationBuckets: make(map[string]map[float64]int64),
		dwellTimeSum:              make(map[string]float64),
		dwellTimeCount:            make(map[string]int64),
		dwellTimeBuckets:          make(map[string]map[float64]int64),
	}
}

func (m *PrometheusMetricsRecorder) RecordConversionResult(ctx context.Context, srcFormat, targetFormat domain.Format, status domain.JobStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s|%s|%s", srcFormat, targetFormat, status)
	m.conversionsTotal[key]++
}

func (m *PrometheusMetricsRecorder) RecordConversionDuration(ctx context.Context, srcFormat, targetFormat domain.Format, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s|%s", srcFormat, targetFormat)
	sec := duration.Seconds()

	m.conversionDurationSum[key] += sec
	m.conversionDurationCount[key]++

	if m.conversionDurationBuckets[key] == nil {
		m.conversionDurationBuckets[key] = make(map[float64]int64)
	}

	for _, le := range defaultBuckets {
		if sec <= le {
			m.conversionDurationBuckets[key][le]++
		}
	}
}

func (m *PrometheusMetricsRecorder) RecordJobDwellTime(ctx context.Context, srcFormat, targetFormat domain.Format, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s|%s", srcFormat, targetFormat)
	sec := duration.Seconds()

	m.dwellTimeSum[key] += sec
	m.dwellTimeCount[key]++

	if m.dwellTimeBuckets[key] == nil {
		m.dwellTimeBuckets[key] = make(map[float64]int64)
	}

	for _, le := range defaultBuckets {
		if sec <= le {
			m.dwellTimeBuckets[key][le]++
		}
	}
}

func (m *PrometheusMetricsRecorder) RecordQueueDepth(ctx context.Context, queueName string, depth int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queueDepth[queueName] = depth
}

func (m *PrometheusMetricsRecorder) RecordEdgeRequest(ctx context.Context, method, endpoint string, statusCode int, errorCode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if errorCode == "" {
		errorCode = "NONE"
	}
	key := fmt.Sprintf("%s|%s|%d|%s", method, endpoint, statusCode, errorCode)
	m.edgeRequestsTotal[key]++
}

// Handler retorna a função http.HandlerFunc para exposição do endpoint /metrics.
func (m *PrometheusMetricsRecorder) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

		fmt.Fprintln(w, "# HELP fileconvert_conversions_total Total de conversões executadas por par de formato e status.")
		fmt.Fprintln(w, "# TYPE fileconvert_conversions_total counter")
		for key, count := range m.conversionsTotal {
			parts := strings.Split(key, "|")
			if len(parts) == 3 {
				fmt.Fprintf(w, "fileconvert_conversions_total{source_format=\"%s\",target_format=\"%s\",status=\"%s\"} %d\n", parts[0], parts[1], parts[2], count)
			}
		}

		fmt.Fprintln(w, "# HELP fileconvert_conversion_duration_seconds Duração da conversão em segundos.")
		fmt.Fprintln(w, "# TYPE fileconvert_conversion_duration_seconds histogram")
		for key, count := range m.conversionDurationCount {
			parts := strings.Split(key, "|")
			if len(parts) == 2 {
				src, target := parts[0], parts[1]
				buckets := m.conversionDurationBuckets[key]
				var cumulative int64
				for _, le := range defaultBuckets {
					cumulative += buckets[le]
					fmt.Fprintf(w, "fileconvert_conversion_duration_seconds_bucket{source_format=\"%s\",target_format=\"%s\",le=\"%.1f\"} %d\n", src, target, le, cumulative)
				}
				fmt.Fprintf(w, "fileconvert_conversion_duration_seconds_bucket{source_format=\"%s\",target_format=\"%s\",le=\"+Inf\"} %d\n", src, target, count)
				sum := m.conversionDurationSum[key]
				fmt.Fprintf(w, "fileconvert_conversion_duration_seconds_sum{source_format=\"%s\",target_format=\"%s\"} %f\n", src, target, sum)
				fmt.Fprintf(w, "fileconvert_conversion_duration_seconds_count{source_format=\"%s\",target_format=\"%s\"} %d\n", src, target, count)
			}
		}

		fmt.Fprintln(w, "# HELP fileconvert_job_queue_dwell_time_seconds Tempo de espera na fila em segundos.")
		fmt.Fprintln(w, "# TYPE fileconvert_job_queue_dwell_time_seconds histogram")
		for key, count := range m.dwellTimeCount {
			parts := strings.Split(key, "|")
			if len(parts) == 2 {
				src, target := parts[0], parts[1]
				buckets := m.dwellTimeBuckets[key]
				var cumulative int64
				for _, le := range defaultBuckets {
					cumulative += buckets[le]
					fmt.Fprintf(w, "fileconvert_job_queue_dwell_time_seconds_bucket{source_format=\"%s\",target_format=\"%s\",le=\"%.1f\"} %d\n", src, target, cumulative)
				}
				fmt.Fprintf(w, "fileconvert_job_queue_dwell_time_seconds_bucket{source_format=\"%s\",target_format=\"%s\",le=\"+Inf\"} %d\n", src, target, count)
				sum := m.dwellTimeSum[key]
				fmt.Fprintf(w, "fileconvert_job_queue_dwell_time_seconds_sum{source_format=\"%s\",target_format=\"%s\"} %f\n", src, target, sum)
				fmt.Fprintf(w, "fileconvert_job_queue_dwell_time_seconds_count{source_format=\"%s\",target_format=\"%s\"} %d\n", src, target, count)
			}
		}

		fmt.Fprintln(w, "# HELP fileconvert_queue_depth Profundidade da fila de jobs pendentes.")
		fmt.Fprintln(w, "# TYPE fileconvert_queue_depth gauge")
		for qName, depth := range m.queueDepth {
			fmt.Fprintf(w, "fileconvert_queue_depth{queue_name=\"%s\"} %d\n", qName, depth)
		}

		fmt.Fprintln(w, "# HELP fileconvert_edge_requests_total Total de requisições na borda HTTP.")
		fmt.Fprintln(w, "# TYPE fileconvert_edge_requests_total counter")
		for key, count := range m.edgeRequestsTotal {
			parts := strings.Split(key, "|")
			if len(parts) == 4 {
				fmt.Fprintf(w, "fileconvert_edge_requests_total{method=\"%s\",endpoint=\"%s\",status_code=\"%s\",error_code=\"%s\"} %d\n", parts[0], parts[1], parts[2], parts[3], count)
			}
		}
	}
}
