package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/swkings/fsm/config"
)

type redisStore struct {
	client *redis.Client
	prefix string
}

func newRedis(cfg config.Redis) (*redisStore, error) {
	if cfg.Address == "" {
		return nil, errors.New("redis address is required")
	}
	if cfg.KeyPrefix == "" {
		cfg.KeyPrefix = "fsm:store"
	}
	backend := &redisStore{client: redis.NewClient(&redis.Options{Addr: cfg.Address, Password: cfg.Password, DB: cfg.DB}), prefix: cfg.KeyPrefix}
	if err := backend.client.Ping(context.Background()).Err(); err != nil {
		_ = backend.client.Close()
		return nil, err
	}
	return backend, nil
}

func (store *redisStore) Descriptor() Descriptor {
	return Descriptor{Driver: config.StoreDriverRedis, Location: store.client.Options().Addr}
}

func (store *redisStore) Save(ctx context.Context, record Record) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	pipeline := store.client.TxPipeline()
	pipeline.HSet(ctx, store.prefix+":records", record.ID, data)
	pipeline.ZAdd(ctx, store.prefix+":index", redis.Z{Score: float64(record.RecordedAt.UnixNano()), Member: record.ID})
	_, err = pipeline.Exec(ctx)
	return err
}

func (store *redisStore) List(ctx context.Context, query Query) ([]Record, error) {
	if query.Limit <= 0 {
		query.Limit = 500
	}
	ids, err := store.client.ZRangeArgs(ctx, redis.ZRangeArgs{Key: store.prefix + ":index", Start: 0, Stop: -1, Rev: true}).Result()
	if err != nil || len(ids) == 0 {
		return []Record{}, err
	}
	values, err := store.client.HMGet(ctx, store.prefix+":records", ids...).Result()
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, value := range values {
		if value == nil {
			continue
		}
		var record Record
		if err = json.Unmarshal([]byte(fmt.Sprint(value)), &record); err != nil {
			return nil, err
		}
		if (query.Key == "" || record.Key == query.Key) && (!query.SuccessfulOnly || record.Success) {
			records = append(records, record)
			if len(records) == query.Limit {
				break
			}
		}
	}
	return records, nil
}

func (store *redisStore) Latest(ctx context.Context, key string) (Record, error) {
	return latest(ctx, store, key)
}

func (store *redisStore) Get(ctx context.Context, id string) (Record, error) {
	data, err := store.client.HGet(ctx, store.prefix+":records", id).Bytes()
	if errors.Is(err, redis.Nil) {
		return Record{}, ErrNotFound
	}
	var record Record
	if err == nil {
		err = json.Unmarshal(data, &record)
	}
	return record, err
}

func (store *redisStore) Delete(ctx context.Context, id string) error {
	pipeline := store.client.TxPipeline()
	deleted := pipeline.HDel(ctx, store.prefix+":records", id)
	pipeline.ZRem(ctx, store.prefix+":index", id)
	_, err := pipeline.Exec(ctx)
	if err == nil && deleted.Val() == 0 {
		err = ErrNotFound
	}
	return err
}
func (store *redisStore) Close() error { return store.client.Close() }
