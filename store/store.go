package store

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
	"github.com/swkings/fsm/config"
)

// New 根据配置创建可用 Store。没有数据库配置时默认创建 JSONL Store。
func New(cfg config.Store) (Store, error) {
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
		if cfg.JSONL.Directory == "" {
			cfg.JSONL.Directory = filepath.Join("data", "fsm")
		}
		if cfg.JSONL.Filename == "" {
			cfg.JSONL.Filename = "fsm.jsonl"
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
	if err := os.MkdirAll(cfg.Directory, 0755); err != nil {
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
	f, err := os.OpenFile(filepath.Join(s.directory, s.filename), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
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
func latest(ctx context.Context, backend Store, key string) (Record, error) {
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

type mysqlStore struct {
	db    *sql.DB
	table string
}

var tableName = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func newMySQL(cfg config.MySQL) (*mysqlStore, error) {
	if cfg.DSN == "" {
		return nil, errors.New("mysql dsn is required")
	}
	if cfg.Table == "" {
		cfg.Table = "fsm_records"
	}
	if !tableName.MatchString(cfg.Table) {
		return nil, errors.New("invalid mysql table")
	}
	db, err := sql.Open("mysql", cfg.DSN)
	if err != nil {
		return nil, err
	}
	backend := &mysqlStore{db: db, table: cfg.Table}
	if err = backend.ready(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return backend, nil
}
func (s *mysqlStore) Descriptor() Descriptor {
	return Descriptor{Driver: config.StoreDriverMySQL, Location: "mysql"}
}
func (s *mysqlStore) ready(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id VARCHAR(64) PRIMARY KEY,fsm_key VARCHAR(255),success BOOLEAN,recorded_at BIGINT,data LONGTEXT,INDEX(fsm_key,success,recorded_at))", s.table))
	return err
}
func (s *mysqlStore) Save(ctx context.Context, record Record) error {
	b, err := json.Marshal(record)
	if err == nil {
		_, err = s.db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id,fsm_key,success,recorded_at,data) VALUES(?,?,?,?,?)", s.table), record.ID, record.Key, record.Success, record.RecordedAt.UnixNano(), b)
	}
	return err
}
func (s *mysqlStore) List(ctx context.Context, query Query) ([]Record, error) {
	if query.Limit <= 0 {
		query.Limit = 500
	}
	q := fmt.Sprintf("SELECT data FROM %s WHERE 1=1", s.table)
	args := []any{}
	if query.Key != "" {
		q += " AND fsm_key=?"
		args = append(args, query.Key)
	}
	if query.SuccessfulOnly {
		q += " AND success=1"
	}
	q += " ORDER BY recorded_at DESC LIMIT ?"
	args = append(args, query.Limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []Record
	for rows.Next() {
		var data []byte
		var record Record
		if err = rows.Scan(&data); err == nil {
			err = json.Unmarshal(data, &record)
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}
func (s *mysqlStore) Latest(ctx context.Context, key string) (Record, error) {
	return latest(ctx, s, key)
}
func (s *mysqlStore) Get(ctx context.Context, id string) (Record, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, fmt.Sprintf("SELECT data FROM %s WHERE id=?", s.table), id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	var record Record
	if err == nil {
		err = json.Unmarshal(data, &record)
	}
	return record, err
}
func (s *mysqlStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id=?", s.table), id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
func (s *mysqlStore) Close() error { return s.db.Close() }

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
func (s *redisStore) Descriptor() Descriptor {
	return Descriptor{Driver: config.StoreDriverRedis, Location: s.client.Options().Addr}
}
func (s *redisStore) Save(ctx context.Context, record Record) error {
	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	pipeline := s.client.TxPipeline()
	pipeline.HSet(ctx, s.prefix+":records", record.ID, b)
	pipeline.ZAdd(ctx, s.prefix+":index", redis.Z{Score: float64(record.RecordedAt.UnixNano()), Member: record.ID})
	_, err = pipeline.Exec(ctx)
	return err
}
func (s *redisStore) List(ctx context.Context, query Query) ([]Record, error) {
	if query.Limit <= 0 {
		query.Limit = 500
	}
	ids, err := s.client.ZRevRange(ctx, s.prefix+":index", 0, -1).Result()
	if err != nil || len(ids) == 0 {
		return []Record{}, err
	}
	values, err := s.client.HMGet(ctx, s.prefix+":records", ids...).Result()
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
func (s *redisStore) Latest(ctx context.Context, key string) (Record, error) {
	return latest(ctx, s, key)
}
func (s *redisStore) Get(ctx context.Context, id string) (Record, error) {
	b, err := s.client.HGet(ctx, s.prefix+":records", id).Bytes()
	if errors.Is(err, redis.Nil) {
		return Record{}, ErrNotFound
	}
	var record Record
	if err == nil {
		err = json.Unmarshal(b, &record)
	}
	return record, err
}
func (s *redisStore) Delete(ctx context.Context, id string) error {
	pipeline := s.client.TxPipeline()
	deleted := pipeline.HDel(ctx, s.prefix+":records", id)
	pipeline.ZRem(ctx, s.prefix+":index", id)
	_, err := pipeline.Exec(ctx)
	if err == nil && deleted.Val() == 0 {
		err = ErrNotFound
	}
	return err
}
func (s *redisStore) Close() error { return s.client.Close() }
