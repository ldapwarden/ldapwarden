package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/ldapwarden/ldapwarden/internal/ldap"
)

// runExpiryMigration moves account expirations onto pwdEndTime.
//
// Earlier versions stored an expiry in pwdAccountLockedTime, which OpenLDAP reads
// as "a lockout began at this instant" — so a scheduled future date disabled the
// account on the spot, and a past one quietly stopped applying. This walks the
// directory and rewrites those values as pwdEndTime, which slapd enforces as an
// actual validity limit.
//
// Reports without writing unless -apply is given: the two ambiguous cases
// (past lock timestamps, shadowExpire) can only be judged by whoever knows what
// the dates meant.
func runExpiryMigration(args []string) error {
	fs := flag.NewFlagSet("migrate-account-expiry", flag.ExitOnError)
	apply := fs.Bool("apply", false, "write the changes (default: report only)")
	includePastLocks := fs.Bool("include-past-locks", false,
		"also migrate pwdAccountLockedTime values in the past — these accounts work today and would start being refused")
	includeShadow := fs.Bool("include-shadow-expire", false,
		"also migrate shadowExpire — moves enforcement from the hosts' PAM/SSSD to the directory")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), `Usage: ldapwarden migrate-account-expiry [flags]

Moves account expirations from pwdAccountLockedTime (and optionally shadowExpire)
to pwdEndTime, the attribute the directory enforces as a validity limit.

Uses the same LDAP_* environment variables as the server.

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	client := ldap.NewClient(cfg.LDAP)
	defer client.Close()

	report, err := client.MigrateAccountExpiry(ldap.ExpiryMigrationOptions{
		Apply:               *apply,
		IncludePastLocks:    *includePastLocks,
		IncludeShadowExpire: *includeShadow,
	})
	if err != nil {
		return err
	}

	for _, entry := range report.Entries {
		switch entry.Action {
		case "migrate":
			verb := "would migrate"
			if report.Applied {
				verb = "migrated"
			}
			log.Printf("%s %s: %s=%s -> pwdEndTime=%s", verb, entry.UID, entry.From, entry.Value, entry.To)
		case "skip":
			log.Printf("skipped %s: %s=%s — %s", entry.UID, entry.From, entry.Value, entry.Reason)
		case "error":
			log.Printf("FAILED %s: %s", entry.UID, entry.Reason)
		}
	}

	if report.Applied {
		log.Printf("done: %d migrated, %d skipped, %d failed", report.Migrated, report.Skipped, report.Failed)
	} else {
		log.Printf("dry run: %d would be migrated, %d skipped. Re-run with -apply to write.",
			report.Migrated, report.Skipped)
	}
	if report.Failed > 0 {
		return fmt.Errorf("%d entries failed to migrate", report.Failed)
	}
	return nil
}
