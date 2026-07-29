package visualization

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
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	ReplayDriverJSONL = "jsonl"
	ReplayDriverMySQL = "mysql"
	ReplayDriverRedis = "redis"
)

var (
	ErrReplaySourceNotFound = errors.New("replay data source not found")
	ErrReplayRecordNotFound = errors.New("replay record not found")
)

type ReplayStorageConfig struct {
	Driver string            `json:"driver,optional"`
	JSONL  ReplayJSONLConfig `json:"jsonl,optional"`
	MySQL  ReplayMySQLConfig `json:"mysql,optional"`
	Redis  ReplayRedisConfig `json:"redis,optional"`
}
type ReplayJSONLConfig struct {
	Directory string `json:"directory,optional"`
	Filename  string `json:"filename,optional"`
}
type ReplayMySQLConfig struct {
	DSN   string `json:"dsn,optional"`
	Table string `json:"table,optional"`
}
type ReplayRedisConfig struct {
	Address   string `json:"address,optional"`
	Password  string `json:"password,optional"`
	DB        int    `json:"db,optional"`
	KeyPrefix string `json:"keyPrefix,optional"`
}
type ReplaySourceConfig struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	ReplayStorageConfig
}
type ReplaySourceView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Driver    string `json:"driver"`
	Location  string `json:"location"`
	Primary   bool   `json:"primary"`
	LastError string `json:"lastError,omitempty"`
}
type ReplayRecord struct {
	ID          string    `json:"id"`
	SourceID    string    `json:"sourceId,omitempty"`
	FSMID       string    `json:"fsmId"`
	DisplayName string    `json:"displayName"`
	Path        string    `json:"path,omitempty"`
	RecordedAt  time.Time `json:"recordedAt"`
	Snapshot    Snapshot  `json:"snapshot"`
}
type ReplayRecordSummary struct {
	ID           string    `json:"id"`
	SourceID     string    `json:"sourceId"`
	FSMID        string    `json:"fsmId"`
	DisplayName  string    `json:"displayName"`
	Path         string    `json:"path,omitempty"`
	TransitionID string    `json:"transitionId,omitempty"`
	State        string    `json:"state"`
	Event        string    `json:"event,omitempty"`
	RecordedAt   time.Time `json:"recordedAt"`
}

type replayStore interface {
	ready(context.Context) error
	save(context.Context, ReplayRecord) error
	list(context.Context, string, int) ([]ReplayRecord, error)
	get(context.Context, string) (ReplayRecord, error)
	remove(context.Context, string) error
	close() error
}
type replaySource struct {
	config           ReplaySourceConfig
	driver, location string
	store            replayStore
	primary          bool
	lastErr          string
	mu               sync.RWMutex
}

func (s *replaySource) fail(err error) {
	s.mu.Lock()
	if err == nil {
		s.lastErr = ""
	} else {
		s.lastErr = err.Error()
	}
	s.mu.Unlock()
}
func (s *replaySource) view() ReplaySourceView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ReplaySourceView{s.config.ID, s.config.Name, s.driver, s.location, s.primary, s.lastErr}
}

type replayHub struct {
	mu       sync.RWMutex
	recordMu sync.Mutex
	sources  map[string]*replaySource
	last     map[string]string
}

func replayDriver(c ReplayStorageConfig) string {
	if v := strings.ToLower(strings.TrimSpace(c.Driver)); v != "" {
		return v
	}
	if c.MySQL.DSN != "" {
		return ReplayDriverMySQL
	}
	if c.Redis.Address != "" {
		return ReplayDriverRedis
	}
	return ReplayDriverJSONL
}
func openReplaySource(c ReplaySourceConfig, primary bool) (*replaySource, error) {
	c.ID = strings.TrimSpace(c.ID)
	if c.ID == "" || strings.ContainsAny(c.ID, "/") {
		return nil, fmt.Errorf("invalid replay source id %q", c.ID)
	}
	if c.Name == "" {
		c.Name = c.ID
	}
	c.Driver = replayDriver(c.ReplayStorageConfig)
	var store replayStore
	location := ""
	var err error
	switch c.Driver {
	case ReplayDriverJSONL:
		if c.JSONL.Directory == "" {
			c.JSONL.Directory = filepath.Join("data", "replay")
		}
		if c.JSONL.Filename == "" {
			c.JSONL.Filename = "fsm-replay.jsonl"
		}
		store = &jsonlStore{dir: c.JSONL.Directory, file: c.JSONL.Filename}
		location = c.JSONL.Directory
	case ReplayDriverMySQL:
		if c.MySQL.DSN == "" {
			return nil, errors.New("mysql dsn is required")
		}
		if c.MySQL.Table == "" {
			c.MySQL.Table = "fsm_replay_records"
		}
		store, err = newMySQLStore(c.MySQL)
		location = "mysql"
	case ReplayDriverRedis:
		if c.Redis.Address == "" {
			return nil, errors.New("redis address is required")
		}
		if c.Redis.KeyPrefix == "" {
			c.Redis.KeyPrefix = "fsm:replay"
		}
		store = newRedisStore(c.Redis)
		location = fmt.Sprintf("%s/%d", c.Redis.Address, c.Redis.DB)
	default:
		return nil, fmt.Errorf("unsupported replay driver %q", c.Driver)
	}
	if err != nil {
		return nil, err
	}
	return &replaySource{config: c, driver: c.Driver, location: location, store: store, primary: primary}, nil
}
func newReplayHub(c ReplayStorageConfig) *replayHub {
	h := &replayHub{sources: map[string]*replaySource{}, last: map[string]string{}}
	s, e := openReplaySource(ReplaySourceConfig{ID: "primary", Name: "Primary replay storage", ReplayStorageConfig: c}, true)
	if e != nil {
		s = &replaySource{config: ReplaySourceConfig{ID: "primary", Name: "Primary replay storage"}, driver: replayDriver(c), primary: true, lastErr: e.Error()}
	}
	h.sources["primary"] = s
	return h
}
func (h *replayHub) source(id string) (*replaySource, error) {
	if id == "" {
		id = "primary"
	}
	h.mu.RLock()
	s, ok := h.sources[id]
	h.mu.RUnlock()
	if !ok || s.store == nil {
		return nil, ErrReplaySourceNotFound
	}
	return s, nil
}
func (h *replayHub) add(ctx context.Context, c ReplaySourceConfig) (ReplaySourceView, error) {
	s, e := openReplaySource(c, false)
	if e != nil {
		return ReplaySourceView{}, e
	}
	if e = s.store.ready(ctx); e != nil {
		_ = s.store.close()
		return ReplaySourceView{}, e
	}
	h.mu.Lock()
	if _, ok := h.sources[c.ID]; ok {
		h.mu.Unlock()
		return ReplaySourceView{}, errors.New("replay source already exists")
	}
	h.sources[c.ID] = s
	h.mu.Unlock()
	return s.view(), nil
}
func (h *replayHub) drop(id string) bool {
	h.mu.Lock()
	s, ok := h.sources[id]
	if !ok || s.primary {
		h.mu.Unlock()
		return false
	}
	delete(h.sources, id)
	h.mu.Unlock()
	_ = s.store.close()
	return true
}
func (h *replayHub) views() []ReplaySourceView {
	h.mu.RLock()
	v := make([]ReplaySourceView, 0, len(h.sources))
	for _, s := range h.sources {
		v = append(v, s.view())
	}
	h.mu.RUnlock()
	sort.Slice(v, func(i, j int) bool {
		if v[i].Primary != v[j].Primary {
			return v[i].Primary
		}
		return v[i].Name < v[j].Name
	})
	return v
}
func (h *replayHub) save(ctx context.Context, snap Snapshot) error {
	fingerprint := snap.TransitionID + "|" + snap.UpdatedAt.UTC().Format(time.RFC3339Nano) + "|" + snap.CurrentState
	h.recordMu.Lock()
	defer h.recordMu.Unlock()
	if h.last[snap.ID] == fingerprint {
		return nil
	}
	s, e := h.source("")
	if e != nil {
		return e
	}
	at := snap.UpdatedAt
	if at.IsZero() {
		at = time.Now()
	}
	e = s.store.save(ctx, ReplayRecord{ID: uuid.NewString(), FSMID: snap.ID, DisplayName: snap.DisplayName, Path: snap.Path, RecordedAt: at, Snapshot: cloneSnapshot(snap)})
	s.fail(e)
	if e == nil {
		h.last[snap.ID] = fingerprint
	}
	return e
}
func (h *replayHub) list(ctx context.Context, source, fsm string, limit int) ([]ReplayRecordSummary, error) {
	s, e := h.source(source)
	if e != nil {
		return nil, e
	}
	r, e := s.store.list(ctx, fsm, limit)
	s.fail(e)
	if e != nil {
		return nil, e
	}
	out := make([]ReplayRecordSummary, 0, len(r))
	for _, x := range r {
		out = append(out, ReplayRecordSummary{x.ID, s.config.ID, x.FSMID, x.DisplayName, x.Path, x.Snapshot.TransitionID, x.Snapshot.CurrentState, x.Snapshot.PreviousEvent, x.RecordedAt})
	}
	return out, nil
}
func (h *replayHub) get(ctx context.Context, source, id string) (ReplayRecord, error) {
	s, e := h.source(source)
	if e != nil {
		return ReplayRecord{}, e
	}
	r, e := s.store.get(ctx, id)
	s.fail(e)
	r.SourceID = s.config.ID
	return r, e
}
func (h *replayHub) remove(ctx context.Context, source, id string) error {
	s, e := h.source(source)
	if e != nil {
		return e
	}
	e = s.store.remove(ctx, id)
	s.fail(e)
	return e
}
func (h *replayHub) close() {
	h.mu.Lock()
	ss := h.sources
	h.sources = map[string]*replaySource{}
	h.mu.Unlock()
	for _, s := range ss {
		if s.store != nil {
			_ = s.store.close()
		}
	}
}

type jsonlStore struct {
	dir, file string
	mu        sync.Mutex
}

func (s *jsonlStore) ready(context.Context) error { return os.MkdirAll(s.dir, 0755) }
func (s *jsonlStore) save(ctx context.Context, r ReplayRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.ready(ctx); e != nil {
		return e
	}
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(filepath.Join(s.dir, s.file), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if e != nil {
		return e
	}
	_, e = f.Write(append(b, byte(10)))
	ce := f.Close()
	if e != nil {
		return e
	}
	return ce
}
func (s *jsonlStore) read() ([]ReplayRecord, error) {
	es, e := os.ReadDir(s.dir)
	if errors.Is(e, os.ErrNotExist) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	var out []ReplayRecord
	for _, x := range es {
		if x.IsDir() || filepath.Ext(x.Name()) != ".jsonl" {
			continue
		}
		f, e := os.Open(filepath.Join(s.dir, x.Name()))
		if e != nil {
			return nil, e
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 65536), 32<<20)
		for sc.Scan() {
			var r ReplayRecord
			if e = json.Unmarshal(sc.Bytes(), &r); e != nil {
				_ = f.Close()
				return nil, e
			}
			out = append(out, r)
		}
		e = sc.Err()
		_ = f.Close()
		if e != nil {
			return nil, e
		}
	}
	return out, nil
}
func (s *jsonlStore) list(ctx context.Context, fsm string, limit int) ([]ReplayRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.read()
	if e != nil {
		return nil, e
	}
	out := r[:0]
	for _, x := range r {
		if fsm == "" || x.FSMID == fsm {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RecordedAt.After(out[j].RecordedAt) })
	if limit <= 0 {
		limit = 500
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, ctx.Err()
}
func (s *jsonlStore) get(ctx context.Context, id string) (ReplayRecord, error) {
	r, e := s.list(ctx, "", 0)
	if e != nil {
		return ReplayRecord{}, e
	}
	for _, x := range r {
		if x.ID == id {
			return x, nil
		}
	}
	return ReplayRecord{}, ErrReplayRecordNotFound
}
func (s *jsonlStore) remove(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return ErrReplayRecordNotFound
	}
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
		path := filepath.Join(s.dir, entry.Name())
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
			var record ReplayRecord
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
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		temp, err := os.CreateTemp(s.dir, ".replay-*.jsonl")
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
		return ErrReplayRecordNotFound
	}
	return nil
}
func (s *jsonlStore) close() error { return nil }

type mysqlStore struct {
	db    *sql.DB
	table string
}

var tableName = regexp.MustCompile("^[A-Za-z0-9_]+$")

func newMySQLStore(c ReplayMySQLConfig) (*mysqlStore, error) {
	if !tableName.MatchString(c.Table) {
		return nil, errors.New("invalid mysql table")
	}
	db, e := sql.Open("mysql", c.DSN)
	return &mysqlStore{db: db, table: c.Table}, e
}
func (s *mysqlStore) ready(ctx context.Context) error {
	_, e := s.db.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id VARCHAR(64) PRIMARY KEY,fsm_id VARCHAR(255),recorded_at BIGINT,data LONGTEXT,INDEX(fsm_id,recorded_at))", s.table))
	return e
}
func (s *mysqlStore) save(ctx context.Context, r ReplayRecord) error {
	if e := s.ready(ctx); e != nil {
		return e
	}
	b, e := json.Marshal(r)
	if e == nil {
		_, e = s.db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s VALUES(?,?,?,?)", s.table), r.ID, r.FSMID, r.RecordedAt.UnixNano(), b)
	}
	return e
}
func (s *mysqlStore) list(ctx context.Context, fsm string, limit int) ([]ReplayRecord, error) {
	if limit <= 0 {
		limit = 500
	}
	q := fmt.Sprintf("SELECT data FROM %s", s.table)
	a := []any{}
	if fsm != "" {
		q += " WHERE fsm_id=?"
		a = append(a, fsm)
	}
	q += " ORDER BY recorded_at DESC LIMIT ?"
	a = append(a, limit)
	rows, e := s.db.QueryContext(ctx, q, a...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []ReplayRecord
	for rows.Next() {
		var b []byte
		var r ReplayRecord
		if e = rows.Scan(&b); e == nil {
			e = json.Unmarshal(b, &r)
		}
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (s *mysqlStore) get(ctx context.Context, id string) (ReplayRecord, error) {
	var b []byte
	e := s.db.QueryRowContext(ctx, fmt.Sprintf("SELECT data FROM %s WHERE id=?", s.table), id).Scan(&b)
	if errors.Is(e, sql.ErrNoRows) {
		return ReplayRecord{}, ErrReplayRecordNotFound
	}
	var r ReplayRecord
	if e == nil {
		e = json.Unmarshal(b, &r)
	}
	return r, e
}
func (s *mysqlStore) remove(ctx context.Context, id string) error {
	r, e := s.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id=?", s.table), id)
	if e != nil {
		return e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return ErrReplayRecordNotFound
	}
	return nil
}
func (s *mysqlStore) close() error { return s.db.Close() }

type redisStore struct {
	c *redis.Client
	p string
}

func newRedisStore(x ReplayRedisConfig) *redisStore {
	return &redisStore{redis.NewClient(&redis.Options{Addr: x.Address, Password: x.Password, DB: x.DB}), x.KeyPrefix}
}
func (s *redisStore) ready(ctx context.Context) error { return s.c.Ping(ctx).Err() }
func (s *redisStore) save(ctx context.Context, r ReplayRecord) error {
	b, e := json.Marshal(r)
	if e != nil {
		return e
	}
	p := s.c.TxPipeline()
	p.HSet(ctx, s.p+":records", r.ID, b)
	p.ZAdd(ctx, s.p+":index", redis.Z{Score: float64(r.RecordedAt.UnixMilli()), Member: r.ID})
	_, e = p.Exec(ctx)
	return e
}
func (s *redisStore) list(ctx context.Context, fsm string, limit int) ([]ReplayRecord, error) {
	if limit <= 0 {
		limit = 500
	}
	ids, e := s.c.ZRevRange(ctx, s.p+":index", 0, int64(limit*4)).Result()
	if e != nil {
		return nil, e
	}
	if len(ids) == 0 {
		return []ReplayRecord{}, nil
	}
	vs, e := s.c.HMGet(ctx, s.p+":records", ids...).Result()
	if e != nil {
		return nil, e
	}
	var out []ReplayRecord
	for _, v := range vs {
		if v == nil {
			continue
		}
		var r ReplayRecord
		if e = json.Unmarshal([]byte(fmt.Sprint(v)), &r); e != nil {
			return nil, e
		}
		if fsm == "" || r.FSMID == fsm {
			out = append(out, r)
			if len(out) == limit {
				break
			}
		}
	}
	return out, nil
}
func (s *redisStore) get(ctx context.Context, id string) (ReplayRecord, error) {
	b, e := s.c.HGet(ctx, s.p+":records", id).Bytes()
	if errors.Is(e, redis.Nil) {
		return ReplayRecord{}, ErrReplayRecordNotFound
	}
	var r ReplayRecord
	if e == nil {
		e = json.Unmarshal(b, &r)
	}
	return r, e
}
func (s *redisStore) remove(ctx context.Context, id string) error {
	p := s.c.TxPipeline()
	n := p.HDel(ctx, s.p+":records", id)
	p.ZRem(ctx, s.p+":index", id)
	_, e := p.Exec(ctx)
	if e == nil && n.Val() == 0 {
		e = ErrReplayRecordNotFound
	}
	return e
}
func (s *redisStore) close() error { return s.c.Close() }
