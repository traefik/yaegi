package zapplug

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// state logs each request with zap (structured logger with its own JSON
// encoder, sampling, object marshalers) and zerolog, into buffers.
type state struct {
	mu   sync.Mutex
	zbuf bytes.Buffer
	zl   *zap.Logger
	rbuf bytes.Buffer
	rl   zerolog.Logger
}

// reqObj is logged as an object through zapcore.ObjectMarshaler.
type reqObj struct{ r *http.Request }

func (o reqObj) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddString("method", o.r.Method)
	enc.AddString("path", o.r.URL.Path)
	return enc.AddArray("headers", zapcore.ArrayMarshalerFunc(func(ae zapcore.ArrayEncoder) error {
		var ks []string
		for k := range o.r.Header {
			if strings.HasPrefix(k, "X-") {
				ks = append(ks, k)
			}
		}
		sort.Strings(ks)
		for _, k := range ks {
			ae.AppendString(k)
		}
		return nil
	}))
}

type syncBuf struct {
	mu *sync.Mutex
	b  *bytes.Buffer
}

func (s syncBuf) Write(p []byte) (int, error) { return s.b.Write(p) }
func (s syncBuf) Sync() error                 { return nil }

func setup(_ context.Context) (*state, error) {
	s := &state{}
	ec := zap.NewProductionEncoderConfig()
	ec.TimeKey = ""
	ec.CallerKey = "caller"
	core := zapcore.NewCore(zapcore.NewJSONEncoder(ec), zapcore.AddSync(syncBuf{&s.mu, &s.zbuf}), zap.DebugLevel)
	s.zl = zap.New(core, zap.AddCaller(), zap.Fields(zap.String("plugin", "complex"))).Named("traefik")
	zerolog.TimestampFunc = func() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
	s.rl = zerolog.New(&s.rbuf).With().Timestamp().Str("plugin", "complex").Logger()
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.zbuf.Reset()
	s.rbuf.Reset()
	l := s.zl.With(zap.Object("req", reqObj{req}))
	l.Info("request", zap.Int("headers", len(req.Header)), zap.Duration("d", 1500*time.Millisecond),
		zap.Strings("query", req.URL.Query()["foo"]), zap.Error(fmt.Errorf("e%d", len(req.URL.Path))))
	if ce := l.Check(zap.DebugLevel, "debug"); ce != nil {
		ce.Write(zap.Bool("checked", true))
	}
	s.rl.Info().Str("method", req.Method).Str("path", req.URL.Path).Dict("h", zerolog.Dict().Int("n", len(req.Header))).
		Err(fmt.Errorf("boom")).Msg("request")
	z := strings.TrimSpace(s.zbuf.String())
	// The caller is file:line of this file: keep only the file.
	z = stripCaller(z)
	return fmt.Sprintf("zap=%s zerolog=%s", z, strings.TrimSpace(s.rbuf.String())), nil
}

func stripCaller(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, `"caller":"`); i >= 0 {
			j := strings.Index(line[i+10:], `"`)
			c := line[i+10 : i+10+j]
			if k := strings.LastIndex(c, ":"); k >= 0 {
				c = c[:k]
			}
			line = line[:i+10] + c + line[i+10+j:]
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
