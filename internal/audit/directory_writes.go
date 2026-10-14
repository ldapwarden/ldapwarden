package audit

import (
	"context"
	"fmt"
	"time"
)

// readOnlyActions are audited actions that write nothing to the directory. They
// must not vouch for a directory write that happened around the same time.
var readOnlyActions = []string{
	string(ActionLogin), string(ActionLoginFailed), string(ActionLogout),
	string(ActionSchemaRefresh), string(ActionUserLockout), string(ActionDirectoryExport),
	string(ActionAccountExpirationNotification), string(ActionPasswordExpirationNotification),
}

// DirectoryWrite is an audited request that wrote to the directory.
type DirectoryWrite struct {
	ID        string
	ActorDN   string
	CreatedAt time.Time
}

// directoryWritesLimit bounds one window. The caller's window is a page of the
// directory's own log, so this is never reached in practice; it keeps a mistake
// from reading the whole table.
const directoryWritesLimit = 50000

// DirectoryWritesBetween lists the audited requests that wrote to the
// directory between from and to, oldest first. Used to tell the writes the
// directory logged that came through this app from the ones that did not.
func (l *Logger) DirectoryWritesBetween(ctx context.Context, from, to time.Time) ([]DirectoryWrite, error) {
	rows, err := l.pool.Query(ctx, `
		SELECT id::text, actor_dn, created_at
		FROM audit_logs
		WHERE created_at BETWEEN $1 AND $2 AND NOT (action = ANY($3))
		ORDER BY created_at
		LIMIT $4`, from, to, readOnlyActions, directoryWritesLimit)
	if err != nil {
		return nil, fmt.Errorf("query audited directory writes: %w", err)
	}
	defer rows.Close()

	var out []DirectoryWrite
	for rows.Next() {
		var w DirectoryWrite
		if err := rows.Scan(&w.ID, &w.ActorDN, &w.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audited directory write: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}
