package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/trace"
)

type Redis struct {
	client *redis.Client
	prefix string
}

var _ Adapter = (*Redis)(nil)

func NewRedis(rawURL, prefix string, dialTimeout time.Duration, provider trace.TracerProvider) (*Redis, error) {
	options, err := redis.ParseURL(rawURL)
	if err != nil {
		return nil, errors.New("invalid Redis URL")
	}
	options.DialTimeout = dialTimeout
	options.ContextTimeoutEnabled = true
	client := redis.NewClient(options)
	if provider != nil {
		err := redisotel.InstrumentTracing(client,
			redisotel.WithTracerProvider(provider),
			redisotel.WithDBStatement(false),
			redisotel.WithCallerEnabled(false),
		)
		if err != nil {
			return nil, errors.Join(fmt.Errorf("instrument Redis: %w", err), client.Close())
		}
	}
	return &Redis{client: client, prefix: prefix}, nil
}

func (a *Redis) Get(ctx context.Context, key string) ([]byte, error) {
	value, err := a.client.Get(ctx, a.prefix+key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, ErrMiss
	}
	return value, err
}

func (a *Redis) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ttl < 0 {
		return ErrInvalidTTL
	}
	return a.client.Set(ctx, a.prefix+key, value, ttl).Err()
}

func (a *Redis) Delete(ctx context.Context, key string) error {
	return a.client.Del(ctx, a.prefix+key).Err()
}

func (a *Redis) Ping(ctx context.Context) error {
	return a.client.Ping(ctx).Err()
}

func (a *Redis) Close() error {
	return a.client.Close()
}
