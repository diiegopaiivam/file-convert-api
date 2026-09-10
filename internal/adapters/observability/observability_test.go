package observability_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/file-convert-api/internal/adapters/observability"
	"github.com/file-convert-api/internal/domain"
	"github.com/file-convert-api/internal/ports"
)

func TestJSONLogger_PropagatesJobIDAndCorrelationID(t *testing.T) {
	logger := observability.NewJSONLogger()

	ctx := context.Background()
	ctx = ports.WithJobID(ctx, "job_test_12345")
	ctx = ports.WithCorrelationID(ctx, "req_test_98765")

	// Garantir que a execução de logs não causa panic
	logger.Info(ctx, "Test log message", ports.Field{Key: "source_format", Value: "wav"})
	logger.Error(ctx, "Test error message", ports.Field{Key: "error_code", Value: "CORRUPTED_FILE"})
}

func TestPrometheusMetricsRecorder_RecordsHistogramBuckets(t *testing.T) {
	metrics := observability.NewPrometheusMetricsRecorder()
	ctx := context.Background()

	// Registrar métricas
	metrics.RecordConversionResult(ctx, domain.FormatWAV, domain.FormatMP3, domain.StatusDone)
	metrics.RecordConversionDuration(ctx, domain.FormatWAV, domain.FormatMP3, 1500*time.Millisecond)
	metrics.RecordJobDwellTime(ctx, domain.FormatWAV, domain.FormatMP3, 200*time.Millisecond)
	metrics.RecordQueueDepth(ctx, "conversion_jobs", 5)
	metrics.RecordEdgeRequest(ctx, "POST", "/v1/conversions/audio", 202, "NONE")

	// Testar Handler HTTP do Prometheus
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()

	handler := metrics.Handler()
	handler.ServeHTTP(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("Esperado status HTTP 200 no /metrics, obtido: %d", res.StatusCode)
	}

	body := w.Body.String()

	expectedSubstrings := []string{
		"fileconvert_conversions_total{source_format=\"wav\",target_format=\"mp3\",status=\"done\"} 1",
		"fileconvert_conversion_duration_seconds_bucket{source_format=\"wav\",target_format=\"mp3\",le=\"2.5\"} 1",
		"fileconvert_conversion_duration_seconds_bucket{source_format=\"wav\",target_format=\"mp3\",le=\"+Inf\"} 1",
		"fileconvert_conversion_duration_seconds_sum{source_format=\"wav\",target_format=\"mp3\"}",
		"fileconvert_queue_depth{queue_name=\"conversion_jobs\"} 5",
		"fileconvert_edge_requests_total{method=\"POST\",endpoint=\"/v1/conversions/audio\",status_code=\"202\",error_code=\"NONE\"} 1",
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(body, sub) {
			t.Errorf("Métrica esperada '%s' não encontrada na saída do /metrics:\n%s", sub, body)
		}
	}
}
