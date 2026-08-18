package migrations_test

import (
	"testing"

	"github.com/golang-migrate/migrate/v4/source/iofs"

	"github.com/ldapwarden/ldapwarden/internal/migrations"
)

// TestEmbeddedMigrations checks the properties golang-migrate will not complain
// about at startup but which break a deployment or a rollback:
//
//   - the embed resolves through the iofs source at all (a bad embed pattern
//     yields an empty FS and a server that silently applies nothing);
//   - versions start at 1 and are contiguous, since a gap means a file was
//     misnamed and is being skipped;
//   - every version has a readable up *and* down, because a missing down file is
//     ignored, and the rollback only fails when someone actually needs it.
//
// It replaces part of what the old "diff -r db/migrations cmd/server/migrations"
// CI job guarded, now that there is a single copy to diff against nothing.
func TestEmbeddedMigrations(t *testing.T) {
	d, err := iofs.New(migrations.FS, migrations.Dir)
	if err != nil {
		t.Fatalf("iofs.New: %v", err)
	}

	first, err := d.First()
	if err != nil {
		t.Fatalf("no migration found in the embedded FS: %v", err)
	}
	if first != 1 {
		t.Errorf("first version = %d, want 1", first)
	}

	version, count := first, 1
	for {
		next, err := d.Next(version)
		if err != nil {
			break // no successor: end of the sequence
		}
		if next != version+1 {
			t.Errorf("gap in the sequence: %d is followed by %d", version, next)
		}
		version, count = next, count+1
	}
	if count < 1 {
		t.Fatal("no migration version read")
	}
	t.Logf("versions 1..%d (%d total)", version, count)

	for v := first; v <= version; v++ {
		if _, _, err := d.ReadUp(v); err != nil {
			t.Errorf("version %d has no readable up: %v", v, err)
		}
		if _, _, err := d.ReadDown(v); err != nil {
			t.Errorf("version %d has no readable down: %v", v, err)
		}
	}
}
