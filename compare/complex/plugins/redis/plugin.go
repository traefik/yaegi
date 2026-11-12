package redisplug

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// state is a rate limiter and a cache on Redis with go-redis: a pipeline,
// a transaction, a Lua script, hashes, sorted sets and pub/sub. The server is
// miniredis, started by the plugin (a pure Go Redis server).
type state struct {
	mr     *miniredis.Miniredis
	rdb    *redis.Client
	script *redis.Script
	sub    *redis.PubSub
}

var incrScript = redis.NewScript(`
local n = redis.call("INCR", KEYS[1])
if n == 1 then redis.call("EXPIRE", KEYS[1], ARGV[1]) end
return n`)

func setup(ctx context.Context) (*state, error) {
	mr := miniredis.NewMiniRedis()
	if err := mr.StartAddr("127.0.0.1:0"); err != nil {
		return nil, err
	}
	s := &state{mr: mr, script: incrScript}
	s.rdb = redis.NewClient(&redis.Options{Addr: mr.Addr(), DialTimeout: 2 * time.Second, Protocol: 2})
	if err := s.rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}
	s.sub = s.rdb.Subscribe(ctx, "events")
	if _, err := s.sub.Receive(ctx); err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		s.sub.Close()
		s.rdb.Close()
		mr.Close()
	}()
	return s, nil
}

func (s *state) process(req *http.Request) (string, error) {
	ctx := req.Context()
	ip, _, _ := net.SplitHostPort(req.RemoteAddr)
	n, err := s.script.Run(ctx, s.rdb, []string{"rl:" + ip}, 60).Int64()
	if err != nil {
		return "", err
	}
	pipe := s.rdb.Pipeline()
	hits := pipe.HIncrBy(ctx, "hits", req.URL.Path, 1)
	pipe.ZIncrBy(ctx, "top", 1, req.Host)
	pipe.Set(ctx, "last", req.Method+" "+req.URL.Path, time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", err
	}
	// Optimistic transaction on a counter.
	err = s.rdb.Watch(ctx, func(tx *redis.Tx) error {
		v, err := tx.Get(ctx, "total").Int()
		if err != nil && err != redis.Nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
			p.Set(ctx, "total", v+1, 0)
			return nil
		})
		return err
	}, "total")
	if err != nil {
		return "", err
	}
	if err := s.rdb.Publish(ctx, "events", req.URL.Path).Err(); err != nil {
		return "", err
	}
	msg, err := s.sub.ReceiveMessage(ctx)
	if err != nil {
		return "", err
	}
	top, err := s.rdb.ZRevRangeWithScores(ctx, "top", 0, 1).Result()
	if err != nil {
		return "", err
	}
	var tops []string
	for _, z := range top {
		tops = append(tops, fmt.Sprintf("%v=%v", z.Member, z.Score))
	}
	total, _ := s.rdb.Get(ctx, "total").Result()
	keys, _ := s.rdb.Keys(ctx, "*").Result()
	var st struct {
		Last string `redis:"last"`
	}
	if err := s.rdb.HSet(ctx, "obj", "last", req.URL.Path).Err(); err != nil {
		return "", err
	}
	if err := s.rdb.HGetAll(ctx, "obj").Scan(&st); err != nil {
		return "", err
	}
	return fmt.Sprintf("rl=%d hits=%d top=%s total=%s pubsub=%s keys=%d scan=%s",
		n, hits.Val(), strings.Join(tops, ","), total, msg.Payload, len(keys), st.Last), nil
}
