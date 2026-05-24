package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func (db *DB) GetAPIKeyByID(ctx context.Context, id int64) (*APIKeyRow, error) {
	rows, err := db.conn.QueryContext(ctx, `SELECT `+apiKeySelectColumns+` FROM api_keys WHERE id=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanAPIKeyRow(rows)
}

func (db *DB) FirstAPIKey(ctx context.Context) (*APIKeyRow, error) {
	rows, err := db.conn.QueryContext(ctx, `SELECT `+apiKeySelectColumns+` FROM api_keys ORDER BY id LIMIT 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanAPIKeyRow(rows)
}

func scanAPIKeyRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*APIKeyRow, error) {
	row := &APIKeyRow{}
	var createdAtRaw, expiresAtRaw, allowedGroupsRaw, limitsRaw interface{}
	if err := scanner.Scan(&row.ID, &row.Name, &row.Key, &createdAtRaw, &row.QuotaLimit, &row.QuotaUsed, &expiresAtRaw, &allowedGroupsRaw, &row.Disabled, &limitsRaw); err != nil {
		return nil, err
	}
	createdAt, err := parseDBTimeValue(createdAtRaw)
	if err != nil {
		return nil, fmt.Errorf("解析 API Key 创建时间失败: %w", err)
	}
	expiresAt, err := parseDBNullTimeValue(expiresAtRaw)
	if err != nil {
		return nil, fmt.Errorf("解析 API Key 过期时间失败: %w", err)
	}
	row.CreatedAt = createdAt
	row.ExpiresAt = expiresAt
	row.AllowedGroupIDs = decodeInt64SliceValue(allowedGroupsRaw)
	row.Limits = decodeAPIKeyLimits(limitsRaw)
	return row, nil
}

func decodeAPIKeyLimits(raw interface{}) APIKeyLimits {
	data := bytesFromDBValue(raw)
	if len(data) == 0 {
		return APIKeyLimits{}
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "{}" || trimmed == "null" {
		return APIKeyLimits{}
	}
	var out APIKeyLimits
	if err := json.Unmarshal(data, &out); err != nil {
		return APIKeyLimits{}
	}
	return out
}

func encodeAPIKeyLimits(l APIKeyLimits) string {
	if l.IsZero() {
		return "{}"
	}
	b, err := json.Marshal(l)
	if err != nil {
		return "{}"
	}
	return string(b)
}
