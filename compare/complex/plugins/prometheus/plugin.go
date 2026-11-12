package prometheusplug

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// state counts requests with the Prometheus client: counters, histograms,
// summaries with labels, a custom collector, gathering into protobuf
// (client_model) and the text exposition format.
type state struct {
	reg     *prometheus.Registry
	reqs    *prometheus.CounterVec
	sizes   *prometheus.HistogramVec
	summary prometheus.Summary
	gauge   prometheus.Gauge
	n       int
}

// collector is a custom collector of the plugin.
type collector struct {
	desc *prometheus.Desc
	s    *state
}

func (c collector) Describe(ch chan<- *prometheus.Desc) { ch <- c.desc }
func (c collector) Collect(ch chan<- prometheus.Metric) {
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, float64(c.s.n*10), "custom")
}

func setup(_ context.Context) (*state, error) {
	s := &state{reg: prometheus.NewRegistry()}
	s.reqs = prometheus.NewCounterVec(prometheus.CounterOpts{Namespace: "plugin", Name: "requests_total", Help: "Requests."},
		[]string{"method", "host"})
	s.sizes = prometheus.NewHistogramVec(prometheus.HistogramOpts{Namespace: "plugin", Name: "path_len", Help: "Path length.",
		Buckets: []float64{1, 5, 10, 20}}, []string{"method"})
	s.summary = prometheus.NewSummary(prometheus.SummaryOpts{Namespace: "plugin", Name: "headers", Help: "Headers.",
		Objectives: map[float64]float64{0.5: 0.05}})
	s.gauge = prometheus.NewGauge(prometheus.GaugeOpts{Namespace: "plugin", Name: "last_len", Help: "Last length."})
	c := collector{desc: prometheus.NewDesc("plugin_custom", "Custom.", []string{"kind"}, prometheus.Labels{"plugin": "complex"}), s: s}
	for _, m := range []prometheus.Collector{s.reqs, s.sizes, s.summary, s.gauge, c} {
		if err := s.reg.Register(m); err != nil {
			return nil, err
		}
	}
	// Registering twice must fail.
	if err := s.reg.Register(s.gauge); err == nil {
		return nil, fmt.Errorf("double registration accepted")
	} else if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
		return nil, fmt.Errorf("unexpected error %T", err)
	}
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	s.n++
	s.reqs.WithLabelValues(req.Method, req.Host).Inc()
	s.sizes.With(prometheus.Labels{"method": req.Method}).Observe(float64(len(req.URL.Path)))
	s.summary.Observe(float64(len(req.Header)))
	s.gauge.Set(float64(len(req.URL.String())))

	mfs, err := s.reg.Gather()
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, mf := range mfs {
		if err := enc.Encode(mf); err != nil {
			return "", err
		}
	}
	lines := 0
	for _, l := range strings.Split(buf.String(), "\n") {
		if l != "" && !strings.HasPrefix(l, "#") {
			lines++
		}
	}
	var hist *dto.Histogram
	for _, mf := range mfs {
		if mf.GetName() == "plugin_path_len" {
			hist = mf.GetMetric()[0].GetHistogram()
		}
	}
	return fmt.Sprintf("families=%d samples=%d bytes=%d count=%v hist=%d/%v custom=%v",
		len(mfs), lines, buf.Len(), testutil.ToFloat64(s.reqs.WithLabelValues(req.Method, req.Host)),
		hist.GetSampleCount(), hist.GetSampleSum(), s.n*10), nil
}
