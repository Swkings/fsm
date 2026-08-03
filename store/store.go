package store

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/swkings/fsm/config"
)

// New 根据配置创建可用 Store。没有数据库配置时默认创建 JSONL Store。
func New(cfg config.Store) (IStore, error) {
	driver := strings.ToLower(strings.TrimSpace(cfg.Driver))
	if driver == "" {
		switch {
		case cfg.MySQL.DSN != "":
			driver = config.StoreDriverMySQL
		case cfg.Redis.Address != "":
			driver = config.StoreDriverRedis
		default:
			driver = config.StoreDriverJSONL
		}
	}
	switch driver {
	case config.StoreDriverJSONL:
		if cfg.JSONL.Filename == "" {
			cfg.JSONL.Filename = "fsm.jsonl"
		}
		if cfg.JSONL.SSH != nil && strings.TrimSpace(cfg.JSONL.SSH.Address) != "" {
			return newSSHJSONL(cfg.JSONL)
		}
		if cfg.JSONL.Directory == "" {
			cfg.JSONL.Directory = filepath.Join("data", "fsm")
		}

		return newJSONL(cfg.JSONL)
	case config.StoreDriverMySQL:
		return newMySQL(cfg.MySQL)
	case config.StoreDriverRedis:
		return newRedis(cfg.Redis)
	default:
		return nil, fmt.Errorf("unsupported fsm store driver %q", driver)
	}
}

type jsonlStore struct {
	directory string
	filename  string
	mu        sync.Mutex
}

func newJSONL(cfg config.JSONL) (*jsonlStore, error) {
	if err := os.MkdirAll(cfg.Directory, 0o755); err != nil {
		return nil, err
	}

	return &jsonlStore{directory: cfg.Directory, filename: cfg.Filename}, nil
}

func (s *jsonlStore) Descriptor() Descriptor {
	return Descriptor{Driver: config.StoreDriverJSONL, Location: s.directory}
}

func (s *jsonlStore) Save(ctx context.Context, record Record) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.directory, s.filename), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}

	return err
}

func (s *jsonlStore) read() ([]Record, error) {
	entries, err := os.ReadDir(s.directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var records []Record
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		file, err := os.Open(filepath.Join(s.directory, entry.Name()))
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 65536), 32<<20)
		for scanner.Scan() {
			var record Record
			if err = json.Unmarshal(scanner.Bytes(), &record); err != nil {
				_ = file.Close()

				return nil, err
			}
			records = append(records, record)
		}
		err = scanner.Err()
		_ = file.Close()
		if err != nil {
			return nil, err
		}
	}

	return records, nil
}

func filter(records []Record, query Query) []Record {
	out := records[:0]
	for _, record := range records {
		if (query.Key == "" || record.Key == query.Key) && (!query.SuccessfulOnly || record.Success) {
			out = append(out, record)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecordedAt.After(out[j].RecordedAt) })
	limit := query.Limit
	if limit <= 0 {
		limit = 500
	}
	if len(out) > limit {
		out = out[:limit]
	}

	return out
}

func (s *jsonlStore) List(ctx context.Context, query Query) ([]Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.read()
	if err != nil {
		return nil, err
	}

	return filter(records, query), ctx.Err()
}

func latest(ctx context.Context, backend IStore, key string) (Record, error) {
	records, err := backend.List(ctx, Query{Key: key, Limit: 1, SuccessfulOnly: true})
	if err != nil {
		return Record{}, err
	}
	if len(records) == 0 {
		return Record{}, ErrNotFound
	}

	return records[0], nil
}

func (s *jsonlStore) Latest(ctx context.Context, key string) (Record, error) {
	return latest(ctx, s, key)
}

func (s *jsonlStore) Get(ctx context.Context, id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := s.read()
	if err != nil {
		return Record{}, err
	}
	for _, record := range records {
		if err = ctx.Err(); err != nil {
			return Record{}, err
		}
		if record.ID == id {
			return record, nil
		}
	}

	return Record{}, ErrNotFound
}

func (s *jsonlStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.directory)
	if err != nil {
		return err
	}
	found := false
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(s.directory, entry.Name())
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		var kept [][]byte
		changed := false
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 65536), 32<<20)
		for scanner.Scan() {
			line := append([]byte(nil), scanner.Bytes()...)
			var record Record
			if err = json.Unmarshal(line, &record); err != nil {
				_ = file.Close()

				return err
			}
			if record.ID == id {
				changed, found = true, true

				continue
			}
			kept = append(kept, line)
		}
		err = scanner.Err()
		_ = file.Close()
		if err != nil || !changed {
			if err != nil {
				return err
			}

			continue
		}
		temp, err := os.CreateTemp(s.directory, ".fsm-*.jsonl")
		if err != nil {
			return err
		}
		tempName := temp.Name()
		for _, line := range kept {
			if _, err = temp.Write(append(line, '\n')); err != nil {
				break
			}
		}
		if closeErr := temp.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(tempName, path)
		}
		if err != nil {
			_ = os.Remove(tempName)

			return err
		}
	}
	if !found {
		return ErrNotFound
	}

	return nil
}
func (s *jsonlStore) Close() error { return nil }
