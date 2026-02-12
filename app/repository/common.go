package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

func isDuplicateEntryError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

func nullableStringValue(v *string) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func nullableInt32Value(v *int32) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func nullableTimeValue(v *time.Time) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

func stringPtrFromNull(v sql.NullString) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}

func int32PtrFromNull(v sql.NullInt32) *int32 {
	if !v.Valid {
		return nil
	}
	n := v.Int32
	return &n
}

func timePtrFromNull(v sql.NullTime) *time.Time {
	if !v.Valid {
		return nil
	}
	t := v.Time
	return &t
}

func serializeMetadata(metadata map[string]string) (string, error) {
	if metadata == nil {
		metadata = map[string]string{}
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func parseMetadata(raw string) (map[string]string, error) {
	if raw == "" {
		return map[string]string{}, nil
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return nil, err
	}
	if metadata == nil {
		metadata = map[string]string{}
	}
	return metadata, nil
}
