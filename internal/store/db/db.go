package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

var (
	ErrConflict = errors.New("conflict")
	ErrNotFound = errors.New("not found")
)

type Pool struct{ db *sql.DB }

func (p *Pool) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return p.db.QueryContext(ctx, query, args...)
}
func (p *Pool) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return p.db.QueryRowContext(ctx, query, args...)
}
func (p *Pool) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return p.db.ExecContext(ctx, query, args...)
}
func (p *Pool) Begin(ctx context.Context) (*Tx, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Tx{tx: tx}, nil
}
func (p *Pool) SQLDB() *sql.DB { return p.db }

type Tx struct{ tx *sql.Tx }

func (t *Tx) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.tx.QueryContext(ctx, query, args...)
}
func (t *Tx) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	return t.tx.QueryRowContext(ctx, query, args...)
}
func (t *Tx) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.tx.ExecContext(ctx, query, args...)
}
func (t *Tx) Commit(context.Context) error   { return t.tx.Commit() }
func (t *Tx) Rollback(context.Context) error { return t.tx.Rollback() }

type Store struct{ pool *Pool }

func New(database *sql.DB) *Store { return &Store{pool: &Pool{db: database}} }
func (s *Store) Pool() *Pool      { return s.pool }

func normalizePage(page, pageSize, defaultPageSize, maxPageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func collectRows[T any](rows *sql.Rows, err error) ([]T, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	plan, err := rowScanPlanFor[T](rows)
	if err != nil {
		return nil, err
	}
	values := make([]T, 0)
	for rows.Next() {
		value, err := scanStructWithPlan[T](rows, plan)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

func one[T any](rows *sql.Rows, err error) (T, error) {
	if err != nil {
		var zero T
		return zero, err
	}
	defer rows.Close()
	if !rows.Next() {
		var zero T
		if err := rows.Err(); err != nil {
			return zero, err
		}
		return zero, ErrNotFound
	}
	plan, err := rowScanPlanFor[T](rows)
	if err != nil {
		var zero T
		return zero, err
	}
	return scanStructWithPlan[T](rows, plan)
}

type rowScanPlan struct {
	columns      []string
	fieldIndexes []int
}

type rowScanPlanKey struct {
	target  reflect.Type
	columns string
}

var rowScanPlans sync.Map

func rowScanPlanFor[T any](rows *sql.Rows) (rowScanPlan, error) {
	columns, err := rows.Columns()
	if err != nil {
		return rowScanPlan{}, err
	}
	target := reflect.TypeOf((*T)(nil)).Elem()
	if target.Kind() != reflect.Struct {
		return rowScanPlan{}, fmt.Errorf("database row target %s is not a struct", target)
	}
	key := rowScanPlanKey{target: target, columns: strings.Join(columns, "\x00")}
	if cached, ok := rowScanPlans.Load(key); ok {
		return cached.(rowScanPlan), nil
	}
	fields := structFields(target)
	indexes := make([]int, len(columns))
	for index, column := range columns {
		indexes[index] = -1
		if fieldIndex, ok := fields[strings.ToLower(column)]; ok {
			indexes[index] = fieldIndex
		}
	}
	plan := rowScanPlan{columns: columns, fieldIndexes: indexes}
	rowScanPlans.Store(key, plan)
	return plan, nil
}

func scanStructWithPlan[T any](rows *sql.Rows, plan rowScanPlan) (T, error) {
	var result T
	raw := make([]any, len(plan.columns))
	targets := make([]any, len(plan.columns))
	for index := range raw {
		targets[index] = &raw[index]
	}
	if err := rows.Scan(targets...); err != nil {
		return result, err
	}
	value := reflect.ValueOf(&result).Elem()
	for index, fieldIndex := range plan.fieldIndexes {
		if fieldIndex < 0 {
			continue
		}
		if err := assignSQLValue(value.Field(fieldIndex), raw[index]); err != nil {
			return result, fmt.Errorf("scan column %s: %w", plan.columns[index], err)
		}
	}
	return result, nil
}

func structFields(t reflect.Type) map[string]int {
	fields := make(map[string]int, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		name := strings.Split(field.Tag.Get("db"), ",")[0]
		if name == "" {
			name = field.Name
		}
		if name != "-" {
			fields[strings.ToLower(name)] = i
		}
	}
	return fields
}

var (
	uuidType    = reflect.TypeOf(uuid.UUID{})
	timeType    = reflect.TypeOf(time.Time{})
	rawJSONType = reflect.TypeOf(json.RawMessage{})
)

func assignSQLValue(dst reflect.Value, src any) error {
	if src == nil {
		return nil
	}
	if dst.Kind() == reflect.Pointer {
		dst.Set(reflect.New(dst.Type().Elem()))
		return assignSQLValue(dst.Elem(), src)
	}
	if dst.Type() == uuidType {
		parsed, err := uuid.Parse(asString(src))
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(parsed))
		return nil
	}
	if dst.Type() == timeType {
		parsed, err := parseTime(src)
		if err != nil {
			return err
		}
		dst.Set(reflect.ValueOf(parsed))
		return nil
	}
	if dst.Type() == rawJSONType {
		dst.SetBytes([]byte(asString(src)))
		return nil
	}
	if dst.Kind() == reflect.Slice {
		return json.Unmarshal([]byte(asString(src)), dst.Addr().Interface())
	}
	switch dst.Kind() {
	case reflect.String:
		dst.SetString(asString(src))
	case reflect.Bool:
		switch value := src.(type) {
		case bool:
			dst.SetBool(value)
		case int64:
			dst.SetBool(value != 0)
		default:
			parsed, err := strconv.ParseBool(asString(src))
			if err != nil {
				return err
			}
			dst.SetBool(parsed)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var value int64
		switch source := src.(type) {
		case int64:
			value = source
		case float64:
			value = int64(source)
		default:
			parsed, err := strconv.ParseInt(asString(src), 10, 64)
			if err != nil {
				return err
			}
			value = parsed
		}
		dst.SetInt(value)
	case reflect.Float32, reflect.Float64:
		var value float64
		switch source := src.(type) {
		case float64:
			value = source
		case int64:
			value = float64(source)
		default:
			parsed, err := strconv.ParseFloat(asString(src), 64)
			if err != nil {
				return err
			}
			value = parsed
		}
		dst.SetFloat(value)
	default:
		return fmt.Errorf("unsupported target type %s", dst.Type())
	}
	return nil
}

func asString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(value)
	}
}

func parseTime(value any) (time.Time, error) {
	if typed, ok := value.(time.Time); ok {
		return typed, nil
	}
	text := asString(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, text); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid time %q", text)
}

func withTx(ctx context.Context, pool *Pool, fn func(*Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func withTxValue[T any](ctx context.Context, pool *Pool, fn func(*Tx) (T, error)) (T, error) {
	tx, err := pool.Begin(ctx)
	if err != nil {
		var zero T
		return zero, err
	}
	defer tx.Rollback(ctx)
	value, err := fn(tx)
	if err != nil {
		var zero T
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		var zero T
		return zero, err
	}
	return value, nil
}
