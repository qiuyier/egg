package cache

import (
	"bytes"
	"context"
	"errors"
	"math"
	"time"

	"github.com/maypok86/otter/v2"
)

type otterValue struct {
	data []byte
	ttl  time.Duration
}

type Otter struct {
	store *otter.Cache[string, otterValue]
}

var _ Adapter = (*Otter)(nil)

func NewOtter(maxEntries int) (*Otter, error) {
	if maxEntries <= 0 {
		return nil, errors.New("otter capacity must be positive")
	}
	store, err := otter.New(&otter.Options[string, otterValue]{
		MaximumSize: maxEntries,
		ExpiryCalculator: otter.ExpiryWritingFunc(func(entry otter.Entry[string, otterValue]) time.Duration {
			return entry.Value.ttl
		}),
	})
	if err != nil {
		return nil, err
	}
	return &Otter{store: store}, nil
}

func (a *Otter) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, ok := a.store.GetIfPresent(key)
	if !ok {
		return nil, ErrMiss
	}
	return bytes.Clone(value.data), nil
}

func (a *Otter) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ttl < 0 {
		return ErrInvalidTTL
	}
	if ttl == 0 {
		ttl = math.MaxInt64
	}
	a.store.Set(key, otterValue{data: bytes.Clone(value), ttl: ttl})
	return nil
}

func (a *Otter) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	a.store.Invalidate(key)
	return nil
}

func (a *Otter) Close() error {
	a.store.StopAllGoroutines()
	return nil
}
