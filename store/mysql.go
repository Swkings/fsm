package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	_ "github.com/go-sql-driver/mysql"
	"github.com/swkings/fsm/config"
)

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

func (store *mysqlStore) Descriptor() Descriptor {
	return Descriptor{Driver: config.StoreDriverMySQL, Location: "mysql"}
}

func (store *mysqlStore) ready(ctx context.Context) error {
	_, err := store.db.ExecContext(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id VARCHAR(64) PRIMARY KEY,fsm_key VARCHAR(255),success BOOLEAN,recorded_at BIGINT,data LONGTEXT,INDEX(fsm_key,success,recorded_at))", store.table))
	return err
}

func (store *mysqlStore) Save(ctx context.Context, record Record) error {
	data, err := json.Marshal(record)
	if err == nil {
		_, err = store.db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id,fsm_key,success,recorded_at,data) VALUES(?,?,?,?,?)", store.table), record.ID, record.Key, record.Success, record.RecordedAt.UnixNano(), data)
	}
	return err
}

func (store *mysqlStore) List(ctx context.Context, query Query) ([]Record, error) {
	if query.Limit <= 0 {
		query.Limit = 500
	}
	statement := fmt.Sprintf("SELECT data FROM %s WHERE 1=1", store.table)
	args := []any{}
	if query.Key != "" {
		statement += " AND fsm_key=?"
		args = append(args, query.Key)
	}
	if query.SuccessfulOnly {
		statement += " AND success=1"
	}
	statement += " ORDER BY recorded_at DESC LIMIT ?"
	args = append(args, query.Limit)
	rows, err := store.db.QueryContext(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
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

func (store *mysqlStore) Latest(ctx context.Context, key string) (Record, error) {
	return latest(ctx, store, key)
}

func (store *mysqlStore) Get(ctx context.Context, id string) (Record, error) {
	var data []byte
	err := store.db.QueryRowContext(ctx, fmt.Sprintf("SELECT data FROM %s WHERE id=?", store.table), id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	var record Record
	if err == nil {
		err = json.Unmarshal(data, &record)
	}
	return record, err
}

func (store *mysqlStore) Delete(ctx context.Context, id string) error {
	result, err := store.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id=?", store.table), id)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
func (store *mysqlStore) Close() error { return store.db.Close() }
