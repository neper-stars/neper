package serial

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
	"github.com/rs/zerolog"
)

//go:embed resources/serialC.txt.gz
var serialKeysGz []byte

// LoadKeys decompresses and returns all serial keys from the embedded file.
// The first line (header "LicenseKey") is skipped.
func LoadKeys() ([]string, error) {
	reader, err := gzip.NewReader(bytes.NewReader(serialKeysGz))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer reader.Close()

	var keys []string
	scanner := bufio.NewScanner(reader)

	// Skip header line
	if scanner.Scan() {
		// First line is "LicenseKey", skip it
	}

	for scanner.Scan() {
		key := scanner.Text()
		if key != "" {
			keys = append(keys, key)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to scan keys: %w", err)
	}

	return keys, nil
}

// LoadKeysIntoDB bulk inserts all serial keys into the serial_key table.
// Uses PostgreSQL COPY for optimal performance.
// This should be called once on first deployment via the load-serials command.
func LoadKeysIntoDB(ctx context.Context, db *sqlx.DB, log *zerolog.Logger) error {
	keys, err := LoadKeys()
	if err != nil {
		return fmt.Errorf("failed to load keys: %w", err)
	}

	log.Info().Int("count", len(keys)).Msg("loaded keys from embedded file")

	// Check if keys are already loaded
	var count int
	if err := db.GetContext(ctx, &count, "SELECT COUNT(*) FROM serial_key"); err != nil {
		return fmt.Errorf("failed to count existing keys: %w", err)
	}

	if count > 0 {
		log.Info().Int("existing", count).Msg("serial keys already loaded, skipping")
		return nil
	}

	// Use PostgreSQL COPY for bulk insert
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(); err != nil && err.Error() != "sql: transaction has already been committed or rolled back" {
			log.Err(err).Msg("failed to rollback transaction")
		}
	}()

	stmt, err := tx.Prepare(pq.CopyIn("serial_key", "key"))
	if err != nil {
		return fmt.Errorf("failed to prepare COPY statement: %w", err)
	}

	start := time.Now()
	for i, key := range keys {
		if _, err := stmt.Exec(key); err != nil {
			return fmt.Errorf("failed to exec COPY for key %d: %w", i, err)
		}

		// Log progress every 100k keys
		if (i+1)%100000 == 0 {
			log.Info().Int("progress", i+1).Msg("inserting keys...")
		}
	}

	if _, err := stmt.Exec(); err != nil {
		return fmt.Errorf("failed to flush COPY: %w", err)
	}

	if err := stmt.Close(); err != nil {
		return fmt.Errorf("failed to close COPY statement: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Info().
		Int("count", len(keys)).
		Dur("duration", time.Since(start)).
		Msg("successfully loaded serial keys into database")

	return nil
}

// AssignKeyToUserTx assigns an available serial key to a user within an existing transaction.
// Returns the assigned key or an error if no keys are available.
// The caller is responsible for committing or rolling back the transaction.
func AssignKeyToUserTx(ctx context.Context, tx *sqlx.Tx, userProfileID string) (string, error) {
	// Select an available key with FOR UPDATE to lock it
	var key string
	err := tx.GetContext(ctx, &key,
		`SELECT key FROM serial_key
		 WHERE user_profile_id IS NULL
		 LIMIT 1
		 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return "", fmt.Errorf("failed to get available key: %w", err)
	}

	// Assign the key to the user
	_, err = tx.ExecContext(ctx,
		`UPDATE serial_key
		 SET user_profile_id = $1, assigned_at = NOW()
		 WHERE key = $2`,
		userProfileID, key)
	if err != nil {
		return "", fmt.Errorf("failed to assign key: %w", err)
	}

	// Update user profile with the serial key
	_, err = tx.ExecContext(ctx,
		`UPDATE user_profile
		 SET serial_key = $1
		 WHERE id = $2`,
		key, userProfileID)
	if err != nil {
		return "", fmt.Errorf("failed to update user profile: %w", err)
	}

	return key, nil
}

// AssignKeyToUser assigns an available serial key to a user.
// Returns the assigned key or an error if no keys are available.
func AssignKeyToUser(ctx context.Context, db *sqlx.DB, userProfileID string) (string, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	key, err := AssignKeyToUserTx(ctx, tx, userProfileID)
	if err != nil {
		return "", err
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("failed to commit transaction: %w", err)
	}

	return key, nil
}

// GetAvailableKeyCount returns the number of unassigned serial keys.
func GetAvailableKeyCount(ctx context.Context, db *sqlx.DB) (int, error) {
	var count int
	err := db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM serial_key WHERE user_profile_id IS NULL`)
	if err != nil {
		return 0, fmt.Errorf("failed to count available keys: %w", err)
	}
	return count, nil
}

// GetAssignedKeyCount returns the number of assigned serial keys.
func GetAssignedKeyCount(ctx context.Context, db *sqlx.DB) (int, error) {
	var count int
	err := db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM serial_key WHERE user_profile_id IS NOT NULL`)
	if err != nil {
		return 0, fmt.Errorf("failed to count assigned keys: %w", err)
	}
	return count, nil
}
