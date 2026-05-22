package database

import (
	"context"
	"database/sql"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"time"
)

const (
	IPBanReasonManual = "manual"
	IPBanReasonQPS    = "qps_limit"
	IPBanReasonRPM    = "rpm_limit"

	IPBanSourceManual = "manual"
	IPBanSourceAuto   = "auto"
)

type IPBanRow struct {
	ID              int64
	IP              string
	Reason          string
	Source          string
	BannedAt        time.Time
	ExpiresAt       sql.NullTime
	UnbannedAt      sql.NullTime
	HitCount        int64
	LastTriggeredAt sql.NullTime
	Enabled         bool
}

type IPBanInput struct {
	IP        string
	Reason    string
	Source    string
	ExpiresAt sql.NullTime
	Enabled   bool
}

func normalizeIPBanReason(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case IPBanReasonQPS, IPBanReasonRPM:
		return value
	default:
		return IPBanReasonManual
	}
}

func normalizeIPBanSource(value string) string {
	value = strings.TrimSpace(value)
	if value == IPBanSourceAuto {
		return IPBanSourceAuto
	}
	return IPBanSourceManual
}

func splitLegacyIPBlacklist(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\t' || r == ' ' || r == ',' || r == ';'
	})
	values := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" || strings.HasPrefix(value, "#") {
			continue
		}
		normalized, err := normalizeIPBanIP(value)
		if err != nil {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		values = append(values, normalized)
	}
	sort.Strings(values)
	return values
}

func normalizeIPBanIP(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("IP 不能为空")
	}
	if strings.Contains(value, "/") {
		return "", fmt.Errorf("IP 黑名单仅支持完整 IP")
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return "", fmt.Errorf("IP 格式无效")
	}
	return addr.String(), nil
}

func (db *DB) CreateIPBan(ctx context.Context, input IPBanInput) (*IPBanRow, error) {
	ip, err := normalizeIPBanIP(input.IP)
	if err != nil {
		return nil, err
	}
	reason := normalizeIPBanReason(input.Reason)
	source := normalizeIPBanSource(input.Source)
	enabled := input.Enabled
	if !enabled {
		enabled = true
	}
	now := time.Now()

	if db.isSQLite() {
		_, err := db.conn.ExecContext(ctx, `
			INSERT INTO ip_bans (ip, reason, source, banned_at, expires_at, hit_count, last_triggered_at, enabled)
			VALUES (?, ?, ?, ?, ?, 1, ?, ?)
			ON CONFLICT(ip) DO UPDATE SET
				reason = excluded.reason,
				source = excluded.source,
				expires_at = excluded.expires_at,
				unbanned_at = NULL,
				hit_count = ip_bans.hit_count + 1,
				last_triggered_at = excluded.last_triggered_at,
				enabled = excluded.enabled
		`, ip, reason, source, db.timeArg(now), nullableTimeArg(input.ExpiresAt), db.timeArg(now), enabled)
		if err != nil {
			return nil, err
		}
		return db.GetIPBanByIP(ctx, ip)
	}

	var id int64
	err = db.conn.QueryRowContext(ctx, `
		INSERT INTO ip_bans (ip, reason, source, banned_at, expires_at, hit_count, last_triggered_at, enabled)
		VALUES ($1, $2, $3, $4, $5, 1, $6, $7)
		ON CONFLICT(ip) DO UPDATE SET
			reason = EXCLUDED.reason,
			source = EXCLUDED.source,
			expires_at = EXCLUDED.expires_at,
			unbanned_at = NULL,
			hit_count = ip_bans.hit_count + 1,
			last_triggered_at = EXCLUDED.last_triggered_at,
			enabled = EXCLUDED.enabled
		RETURNING id
	`, ip, reason, source, db.timeArg(now), nullableTimeArg(input.ExpiresAt), db.timeArg(now), enabled).Scan(&id)
	if err != nil {
		return nil, err
	}
	return db.GetIPBanByID(ctx, id)
}

func (db *DB) RecordAutoIPBan(ctx context.Context, ip string, reason string, expiresAt time.Time) (*IPBanRow, error) {
	return db.CreateIPBan(ctx, IPBanInput{
		IP:        ip,
		Reason:    reason,
		Source:    IPBanSourceAuto,
		ExpiresAt: sql.NullTime{Time: expiresAt, Valid: !expiresAt.IsZero()},
		Enabled:   true,
	})
}

func (db *DB) GetIPBanByIP(ctx context.Context, ip string) (*IPBanRow, error) {
	query := `
		SELECT id, ip, reason, source, banned_at, expires_at, unbanned_at, hit_count, last_triggered_at, enabled
		FROM ip_bans WHERE ip = $1
	`
	if db.isSQLite() {
		query = strings.ReplaceAll(query, "$1", "?")
	}
	return db.scanIPBanRow(db.conn.QueryRowContext(ctx, query, ip))
}

func (db *DB) GetIPBanByID(ctx context.Context, id int64) (*IPBanRow, error) {
	query := `
		SELECT id, ip, reason, source, banned_at, expires_at, unbanned_at, hit_count, last_triggered_at, enabled
		FROM ip_bans WHERE id = $1
	`
	if db.isSQLite() {
		query = strings.ReplaceAll(query, "$1", "?")
	}
	return db.scanIPBanRow(db.conn.QueryRowContext(ctx, query, id))
}

func (db *DB) scanIPBanRow(scanner interface {
	Scan(dest ...interface{}) error
}) (*IPBanRow, error) {
	var row IPBanRow
	var bannedAtRaw, expiresAtRaw, unbannedAtRaw, lastTriggeredAtRaw interface{}
	if err := scanner.Scan(
		&row.ID, &row.IP, &row.Reason, &row.Source,
		&bannedAtRaw, &expiresAtRaw, &unbannedAtRaw,
		&row.HitCount, &lastTriggeredAtRaw, &row.Enabled,
	); err != nil {
		return nil, err
	}
	bannedAt, err := parseDBTimeValue(bannedAtRaw)
	if err != nil {
		return nil, err
	}
	row.BannedAt = bannedAt
	if row.ExpiresAt, err = parseDBNullTimeValue(expiresAtRaw); err != nil {
		return nil, err
	}
	if row.UnbannedAt, err = parseDBNullTimeValue(unbannedAtRaw); err != nil {
		return nil, err
	}
	if row.LastTriggeredAt, err = parseDBNullTimeValue(lastTriggeredAtRaw); err != nil {
		return nil, err
	}
	return &row, nil
}

func (db *DB) ListIPBans(ctx context.Context, includeInactive bool) ([]IPBanRow, error) {
	where := ""
	args := []interface{}{}
	if !includeInactive {
		where = `WHERE enabled = $1 AND unbanned_at IS NULL AND (expires_at IS NULL OR expires_at > $2)`
		args = append(args, true, db.timeArg(time.Now()))
	}
	query := `
		SELECT id, ip, reason, source, banned_at, expires_at, unbanned_at, hit_count, last_triggered_at, enabled
		FROM ip_bans ` + where + `
		ORDER BY ip ASC, id ASC
	`
	if db.isSQLite() {
		query = strings.ReplaceAll(query, "$1", "?")
		query = strings.ReplaceAll(query, "$2", "?")
	}
	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := []IPBanRow{}
	for rows.Next() {
		row, err := db.scanIPBanRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *row)
	}
	return result, rows.Err()
}

func (db *DB) UnbanIPBan(ctx context.Context, id int64) error {
	query := `UPDATE ip_bans SET enabled = $1, unbanned_at = $2 WHERE id = $3`
	args := []interface{}{false, db.timeArg(time.Now()), id}
	if db.isSQLite() {
		query = `UPDATE ip_bans SET enabled = ?, unbanned_at = ? WHERE id = ?`
	}
	_, err := db.conn.ExecContext(ctx, query, args...)
	return err
}

func (db *DB) DeleteIPBan(ctx context.Context, id int64) error {
	query := `DELETE FROM ip_bans WHERE id = $1`
	if db.isSQLite() {
		query = `DELETE FROM ip_bans WHERE id = ?`
	}
	_, err := db.conn.ExecContext(ctx, query, id)
	return err
}

func (db *DB) migrateLegacyIPBlacklist(ctx context.Context) error {
	if db.isSQLite() {
		columns, err := db.sqliteTableColumns(ctx, "system_settings")
		if err != nil {
			return err
		}
		if _, ok := columns["ip_blacklist"]; !ok {
			return nil
		}
		var raw sql.NullString
		if err := db.conn.QueryRowContext(ctx, `SELECT COALESCE(ip_blacklist, '') FROM system_settings WHERE id = 1`).Scan(&raw); err != nil && !errorsIsNoRows(err) {
			return err
		}
		for _, ip := range splitLegacyIPBlacklist(raw.String) {
			if _, err := db.CreateIPBan(ctx, IPBanInput{IP: ip, Reason: IPBanReasonManual, Source: IPBanSourceManual, Enabled: true}); err != nil {
				return err
			}
		}
		_, err = db.conn.ExecContext(ctx, `ALTER TABLE system_settings DROP COLUMN ip_blacklist`)
		return err
	}

	var exists bool
	if err := db.conn.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_name = 'system_settings' AND column_name = 'ip_blacklist'
		)
	`).Scan(&exists); err != nil {
		return err
	}
	if exists {
		var raw sql.NullString
		if err := db.conn.QueryRowContext(ctx, `SELECT COALESCE(ip_blacklist, '') FROM system_settings WHERE id = 1`).Scan(&raw); err != nil && !errorsIsNoRows(err) {
			return err
		}
		for _, ip := range splitLegacyIPBlacklist(raw.String) {
			if _, err := db.CreateIPBan(ctx, IPBanInput{IP: ip, Reason: IPBanReasonManual, Source: IPBanSourceManual, Enabled: true}); err != nil {
				return err
			}
		}
		_, err := db.conn.ExecContext(ctx, `ALTER TABLE system_settings DROP COLUMN IF EXISTS ip_blacklist`)
		return err
	}
	return nil
}

func errorsIsNoRows(err error) bool {
	return err == sql.ErrNoRows
}
