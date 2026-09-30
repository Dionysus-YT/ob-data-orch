package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ob-data-orch/migrations"
)

var migrationNamePattern = regexp.MustCompile(`^(\d{4})_([a-z0-9_]+)\.sql$`)

type migration struct {
	version  int
	name     string
	checksum string
	sql      string
}

func Apply(ctx context.Context, db *sql.DB) error {
	return ApplyFS(ctx, db, migrations.Files)
}

// ApplyFS exists so the migration rules can be tested with synthetic files.
func ApplyFS(ctx context.Context, db *sql.DB, migrationFiles fs.FS) error {
	if db == nil {
		return errors.New("migration database is nil")
	}
	if err := requireForeignKeys(ctx, db); err != nil {
		return err
	}
	if err := quickCheck(ctx, db); err != nil {
		return fmt.Errorf("pre-migration integrity check failed: %w", err)
	}

	loaded, err := loadMigrations(migrationFiles)
	if err != nil {
		return err
	}
	applied, tableExists, err := loadApplied(ctx, db)
	if err != nil {
		return err
	}
	if !tableExists && len(applied) != 0 {
		return errors.New("migration history is inconsistent")
	}

	latestSupported := loaded[len(loaded)-1].version
	for version := range applied {
		if version > latestSupported {
			return fmt.Errorf("database schema version %04d is newer than supported version %04d", version, latestSupported)
		}
	}

	for _, item := range loaded {
		if checksum, ok := applied[item.version]; ok {
			if checksum != item.checksum {
				return fmt.Errorf("migration %04d checksum mismatch", item.version)
			}
			continue
		}
		if item.version != 1 && !tableExists {
			return fmt.Errorf("migration %04d cannot initialize an empty database", item.version)
		}
		if err := applyOne(ctx, db, item); err != nil {
			return err
		}
		tableExists = true
	}

	if err := quickCheck(ctx, db); err != nil {
		return fmt.Errorf("post-migration integrity check failed: %w", err)
	}
	return nil
}

func loadMigrations(migrationFiles fs.FS) ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	items := make([]migration, 0, len(entries))
	seen := make(map[int]struct{})
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		match := migrationNamePattern.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(match[1])
		if err != nil || version < 1 {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}
		if _, exists := seen[version]; exists {
			return nil, fmt.Errorf("duplicate migration version %04d", version)
		}
		seen[version] = struct{}{}
		content, err := fs.ReadFile(migrationFiles, entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %04d: %w", version, err)
		}
		digest := sha256.Sum256(content)
		items = append(items, migration{
			version:  version,
			name:     match[2],
			checksum: hex.EncodeToString(digest[:]),
			sql:      string(content),
		})
	}
	if len(items) == 0 {
		return nil, errors.New("no migrations found")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].version < items[j].version })
	for index, item := range items {
		want := index + 1
		if item.version != want {
			return nil, fmt.Errorf("migration sequence gap: found %04d, want %04d", item.version, want)
		}
	}
	return items, nil
}

func loadApplied(ctx context.Context, db *sql.DB) (map[int]string, bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`).Scan(&count); err != nil {
		return nil, false, fmt.Errorf("check migration table: %w", err)
	}
	result := make(map[int]string)
	if count == 0 {
		return result, false, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT version, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, true, fmt.Errorf("read migration history: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var version int
		var checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			return nil, true, fmt.Errorf("scan migration history: %w", err)
		}
		result[version] = checksum
	}
	if err := rows.Err(); err != nil {
		return nil, true, fmt.Errorf("iterate migration history: %w", err)
	}
	return result, true, nil
}

func applyOne(ctx context.Context, db *sql.DB, item migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %04d: %w", item.version, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, item.sql); err != nil {
		return fmt.Errorf("apply migration %04d: %w", item.version, err)
	}
	// 表重建可延迟外键检查，但只能在整库引用完整时清除旧表的延迟计数。
	// 始终保留 foreign_keys=ON，任何悬空引用都回滚整个迁移。
	rows, err := tx.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("check migration foreign keys: %w", err)
	}
	invalid := rows.Next()
	checkErr := rows.Err()
	rows.Close()
	if invalid || checkErr != nil {
		return fmt.Errorf("migration %04d foreign key integrity failed", item.version)
	}
	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = OFF`); err != nil {
		return fmt.Errorf("finish migration foreign key check: %w", err)
	}
	appliedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
		item.version, item.name, item.checksum, appliedAt,
	); err != nil {
		return fmt.Errorf("record migration %04d: %w", item.version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %04d: %w", item.version, err)
	}
	return nil
}

func requireForeignKeys(ctx context.Context, db *sql.DB) error {
	var enabled int
	if err := db.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		return fmt.Errorf("read foreign key setting: %w", err)
	}
	if enabled != 1 {
		return errors.New("SQLite foreign_keys must be enabled before migration")
	}
	return nil
}

func quickCheck(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `PRAGMA quick_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var failures []string
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			return err
		}
		if !strings.EqualFold(result, "ok") {
			failures = append(failures, result)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(failures) > 0 {
		return fmt.Errorf("SQLite quick_check returned %d failure(s)", len(failures))
	}
	return nil
}
