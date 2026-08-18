// Package migrations holds the SQL schema migrations as the single source of
// truth for the schema.
//
// They used to live in two places — db/migrations, applied by the integration
// tests off the filesystem, and cmd/server/migrations, embedded in the binary —
// because go:embed cannot reach outside its own package directory. Nothing but a
// CI "diff -r" stopped the two copies from drifting, and a drift meant the tests
// passed against a schema the binary never creates. One embedded copy, consumed
// by both the server and the tests, removes the failure mode instead of policing
// it.
package migrations

import "embed"

// FS holds the .sql files. Use it with golang-migrate's iofs source, passing
// Dir as the path.
//
//go:embed *.sql
var FS embed.FS

// Dir is the path of the .sql files inside FS. They sit at its root, so this is
// simply ".", but naming it keeps the iofs call sites from hardcoding a bare dot.
const Dir = "."
