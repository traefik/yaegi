package otelplug

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// state traces each request with the OpenTelemetry SDK: spans in memory and
// sent with OTLP/HTTP (protobuf) to a mock collector, W3C propagation to
// next, and metrics read by a manual reader.
type state struct {
	tp       *sdktrace.TracerProvider
	mem      *tracetest.InMemoryExporter
	reader   *sdkmetric.ManualReader
	counter  metric.Int64Counter
	hist     metric.Float64Histogram
	prop     propagation.TextMapPropagator
	srv      *httptest.Server
	received atomic.Int64 // bytes received by the collector
}

// ids gives trace and span ids that do not change between runs.
type ids struct {
	mu sync.Mutex
	n  uint64
}

func (g *ids) next() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return g.n
}

func (g *ids) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	var t trace.TraceID
	binary.BigEndian.PutUint64(t[8:], g.next()+0x1000)
	return t, g.NewSpanID(ctx, t)
}

func (g *ids) NewSpanID(context.Context, trace.TraceID) trace.SpanID {
	var s trace.SpanID
	binary.BigEndian.PutUint64(s[:], g.next())
	return s
}

func setup(ctx context.Context) (*state, error) {
	s := &state{mem: tracetest.NewInMemoryExporter(), reader: sdkmetric.NewManualReader()}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		if r.URL.Path == "/v1/traces" && r.Header.Get("Content-Type") == "application/x-protobuf" {
			s.received.Add(int64(len(b)))
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(s.srv.URL+"/v1/traces"),
		otlptracehttp.WithCompression(otlptracehttp.NoCompression))
	if err != nil {
		return nil, err
	}
	res, err := resource.Merge(resource.Empty(), resource.NewSchemaless(
		semconv.ServiceName("traefik-plugin"), attribute.String("plugin", "otel")))
	if err != nil {
		return nil, err
	}
	s.tp = sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithIDGenerator(&ids{}),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithSyncer(s.mem),
		sdktrace.WithSyncer(exp),
	)
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(s.reader), sdkmetric.WithResource(res))
	meter := mp.Meter("complex")
	if s.counter, err = meter.Int64Counter("requests", metric.WithDescription("requests seen")); err != nil {
		return nil, err
	}
	if s.hist, err = meter.Float64Histogram("path_len", metric.WithExplicitBucketBoundaries(1, 5, 10, 20)); err != nil {
		return nil, err
	}
	s.prop = propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	go func() {
		<-ctx.Done()
		s.tp.Shutdown(context.Background())
		s.srv.Close()
	}()
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	ctx := s.prop.Extract(req.Context(), propagation.HeaderCarrier(req.Header))
	tracer := s.tp.Tracer("complex/otel")
	ctx, span := tracer.Start(ctx, req.Method+" "+req.URL.Path,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithTimestamp(time.Unix(1_700_000_000, 0)),
		trace.WithAttributes(semconv.HTTPRequestMethodKey.String(req.Method), attribute.String("host", req.Host)))
	_, child := tracer.Start(ctx, "child")
	child.AddEvent("work", trace.WithAttributes(attribute.Int("n", len(req.Header))))
	child.End()
	if strings.Contains(req.URL.Path, "admin") {
		span.SetStatus(codes.Error, "admin")
	}
	attrs := metric.WithAttributes(attribute.String("method", req.Method))
	s.counter.Add(ctx, 1, attrs)
	s.hist.Record(ctx, float64(len(req.URL.Path)), attrs)
	s.prop.Inject(ctx, propagation.HeaderCarrier(req.Header))
	span.End(trace.WithTimestamp(time.Unix(1_700_000_001, 0)))

	spans := s.mem.GetSpans()
	last := spans[len(spans)-1]
	var rm metricdata.ResourceMetrics
	if err := s.reader.Collect(ctx, &rm); err != nil {
		return "", err
	}
	var ms []string
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch d := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, p := range d.DataPoints {
					v, _ := p.Attributes.Value("method")
					ms = append(ms, fmt.Sprintf("%s{%s}=%d", m.Name, v.AsString(), p.Value))
				}
			case metricdata.Histogram[float64]:
				for _, p := range d.DataPoints {
					v, _ := p.Attributes.Value("method")
					ms = append(ms, fmt.Sprintf("%s{%s}=%d/%v", m.Name, v.AsString(), p.Count, p.BucketCounts))
				}
			}
		}
	}
	sort.Strings(ms)
	return fmt.Sprintf("spans=%d last=%s kind=%s status=%s dur=%s parent=%t traceparent=%s metrics=%v otlp=%t",
		len(spans), last.Name, last.SpanKind, last.Status.Code, last.EndTime.Sub(last.StartTime),
		last.Parent.IsValid(), req.Header.Get("Traceparent"), ms, s.received.Load() > 0), nil
}
