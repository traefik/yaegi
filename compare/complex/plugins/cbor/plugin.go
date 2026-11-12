package cborplug

import (
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/vmihailenco/msgpack/v5"
)

// state encodes plugin types with two binary codecs that work by reflect:
// CBOR (fxamacker/cbor: tags, keyasint, toarray, canonical mode) and
// MessagePack (vmihailenco/msgpack: custom encoders).
type state struct {
	em cbor.EncMode
	dm cbor.DecMode
}

// Event uses the struct tags of both codecs.
type Event struct {
	_       struct{}          `cbor:",toarray"`
	Method  string            `msgpack:"m"`
	Path    string            `msgpack:"p"`
	Headers map[string]string `msgpack:"h,omitempty"`
	At      time.Time         `msgpack:"t"`
	Score   Score             `msgpack:"s"`
}

// Small uses integer keys.
type Small struct {
	A int    `cbor:"1,keyasint"`
	B string `cbor:"2,keyasint,omitempty"`
}

// Score has its own MessagePack encoding.
type Score int

func (s Score) EncodeMsgpack(enc *msgpack.Encoder) error { return enc.EncodeString(fmt.Sprintf("score-%d", int(s))) }
func (s *Score) DecodeMsgpack(dec *msgpack.Decoder) error {
	str, err := dec.DecodeString()
	if err != nil {
		return err
	}
	_, err = fmt.Sscanf(str, "score-%d", (*int)(s))
	return err
}

func setup(_ context.Context) (*state, error) {
	opts := cbor.CanonicalEncOptions()
	opts.Time = cbor.TimeRFC3339Nano
	em, err := opts.EncMode()
	if err != nil {
		return nil, err
	}
	dm, err := cbor.DecOptions{DupMapKey: cbor.DupMapKeyEnforcedAPF}.DecMode()
	if err != nil {
		return nil, err
	}
	return &state{em: em, dm: dm}, nil
}

func (s *state) process(req *http.Request) (string, error) {
	ev := Event{Method: req.Method, Path: req.URL.Path, Headers: map[string]string{}, At: time.Unix(1_700_000_000, 5).UTC(),
		Score: Score(len(req.Header))}
	for k := range req.Header {
		ev.Headers[k] = req.Header.Get(k)
	}
	cb, err := s.em.Marshal(ev)
	if err != nil {
		return "", err
	}
	var back Event
	if err := s.dm.Unmarshal(cb, &back); err != nil {
		return "", err
	}
	sb, err := s.em.Marshal(Small{A: len(req.URL.Path)})
	if err != nil {
		return "", err
	}
	var diag string
	if diag, err = cbor.Diagnose(sb); err != nil {
		return "", err
	}
	mb, err := msgpack.Marshal(&ev)
	if err != nil {
		return "", err
	}
	var mback Event
	if err := msgpack.Unmarshal(mb, &mback); err != nil {
		return "", err
	}
	var generic map[string]any
	if err := msgpack.Unmarshal(mb, &generic); err != nil {
		return "", err
	}
	return fmt.Sprintf("cbor=%d head=%s back=%s %s %d at=%s small=%s diag=%s msgpack=%d score=%d/%v",
		len(cb), hex.EncodeToString(cb[:4]), back.Method, back.Path, len(back.Headers), back.At.Format(time.RFC3339Nano),
		hex.EncodeToString(sb), diag, len(mb), mback.Score, generic["s"]), nil
}
