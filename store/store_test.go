package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/swkings/fsm/config"
)

func TestJSONLStoreLifecycle(t *testing.T) {
	backend, err := New(config.Store{JSONL: config.JSONL{Directory: t.TempDir(), Filename: "records.jsonl"}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	testStoreLifecycle(t, backend)
}

func TestMySQLStoreLifecycle(t *testing.T) {
	dsn, configured := os.LookupEnv("FSM_TEST_MYSQL_DSN")
	if !configured {
		dsn = "root@tcp(127.0.0.1:3306)/test?parseTime=true&timeout=2s&readTimeout=2s&writeTimeout=2s"
	}
	table := fmt.Sprintf("fsm_store_test_%d", time.Now().UnixNano())
	backend, err := New(config.Store{
		Driver: config.StoreDriverMySQL,
		MySQL:  config.MySQL{DSN: dsn, Table: table},
	})
	if err != nil {
		handleUnavailableIntegration(t, "mysql", configured, err)
	}
	mysqlBackend := backend.(*mysqlStore)
	t.Cleanup(func() {
		_, _ = mysqlBackend.db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)
		_ = backend.Close()
	})
	if got := backend.Descriptor().Driver; got != config.StoreDriverMySQL {
		t.Fatalf("driver=%q", got)
	}
	testStoreLifecycle(t, backend)
}

func TestRedisStoreLifecycle(t *testing.T) {
	address, configured := os.LookupEnv("FSM_TEST_REDIS_ADDRESS")
	if !configured {
		address = "127.0.0.1:6379"
	}
	database := 0
	if raw := os.Getenv("FSM_TEST_REDIS_DB"); raw != "" {
		var err error
		database, err = strconv.Atoi(raw)
		if err != nil {
			t.Fatalf("FSM_TEST_REDIS_DB=%q: %v", raw, err)
		}
	}
	prefix := fmt.Sprintf("fsm:store:test:%d", time.Now().UnixNano())
	backend, err := New(config.Store{
		Driver: config.StoreDriverRedis,
		Redis: config.Redis{
			Address:   address,
			Password:  os.Getenv("FSM_TEST_REDIS_PASSWORD"),
			DB:        database,
			KeyPrefix: prefix,
		},
	})
	if err != nil {
		handleUnavailableIntegration(t, "redis", configured, err)
	}
	redisBackend := backend.(*redisStore)
	t.Cleanup(func() {
		_ = redisBackend.client.Del(context.Background(), prefix+":records", prefix+":index").Err()
		_ = backend.Close()
	})
	if got := backend.Descriptor().Driver; got != config.StoreDriverRedis {
		t.Fatalf("driver=%q", got)
	}
	testStoreLifecycle(t, backend)
}

func testStoreLifecycle(t *testing.T, backend Store) {
	t.Helper()
	suffix := fmt.Sprintf("%s-%d", strings.ToLower(t.Name()), time.Now().UnixNano())
	record := Record{
		ID:          "record-" + suffix,
		Key:         "player/" + suffix,
		DisplayName: "integration-player",
		Category:    "tests",
		RecordedAt:  time.Now().UTC(),
		Success:     true,
		Metadata:    Metadata{CurrentState: "RUN"},
		Trace:       Trace{FromState: "IDLE", Event: "RUN", ToState: "RUN", FinalState: "RUN", Success: true},
	}
	ctx := context.Background()
	if err := backend.Save(ctx, record); err != nil {
		t.Fatalf("Save: %v", err)
	}
	latest, err := backend.Latest(ctx, record.Key)
	if err != nil || latest.ID != record.ID {
		t.Fatalf("Latest: record=%+v err=%v", latest, err)
	}
	listed, err := backend.List(ctx, Query{Key: record.Key, Limit: 1, SuccessfulOnly: true})
	if err != nil || len(listed) != 1 || listed[0].ID != record.ID {
		t.Fatalf("List: records=%+v err=%v", listed, err)
	}
	got, err := backend.Get(ctx, record.ID)
	if err != nil || got.Metadata.CurrentState != "RUN" || got.Trace.Event != "RUN" {
		t.Fatalf("Get: record=%+v err=%v", got, err)
	}
	if err = backend.Delete(ctx, record.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err = backend.Get(ctx, record.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after delete: %v", err)
	}
}

func handleUnavailableIntegration(t *testing.T, backend string, configured bool, err error) {
	t.Helper()
	if configured {
		t.Fatalf("connect configured %s store: %v", backend, err)
	}
	t.Skipf("local %s integration is unavailable: %v", backend, err)
}

func TestStoreDefaultsToJSONL(t *testing.T) {
	backend, err := New(config.Store{JSONL: config.JSONL{Directory: t.TempDir()}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = backend.Close() })
	if got := backend.Descriptor().Driver; got != config.StoreDriverJSONL {
		t.Fatalf("driver=%q", got)
	}
}
