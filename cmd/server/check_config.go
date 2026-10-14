package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/ldapwarden/ldapwarden/internal/config"
	"github.com/ldapwarden/ldapwarden/internal/preflight"
)

// errCheckFailed makes the process exit non-zero once the report has been
// printed; the report is the explanation, so nothing is added to it.
var errCheckFailed = fmt.Errorf("configuration check failed")

// runCheckConfig validates the configuration the server would start with.
//
// Exit status 0 when nothing is an error (warnings included), 1 otherwise, so it
// can gate a deployment. Reads the same environment as the server; nothing is
// written anywhere.
func runCheckConfig(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("check-config", flag.ExitOnError)
	connect := fs.Bool("connect", false, "also connect to PostgreSQL and the directory, as the server would")
	asJSON := fs.Bool("json", false, "print the findings as JSON")
	fs.Usage = func() {
		_, _ = fmt.Fprintf(fs.Output(), `Usage: ldapwarden check-config [flags]

Checks the configuration the server would start with: secrets and secret
files, TLS settings and certificate files, scheduled-task specs and the other
settings the server only discovers at run time. With -connect it also binds to
the directory and checks what the configuration names there: base entries, the
admin group and the default password policy.

Uses the same environment variables as the server. Exits 1 when any finding is
an error.

Flags:
`)
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, loadErr := config.Load()
	report := preflight.Static(cfg, loadErr)
	if *connect {
		preflight.Connected(context.Background(), report, cfg)
	}

	if *asJSON {
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(struct {
			OK bool `json:"ok"`
			*preflight.Report
		}{OK: !report.Failed(), Report: report}); err != nil {
			return err
		}
	} else {
		for _, f := range report.Findings {
			_, _ = fmt.Fprintf(stdout, "%-5s  %-8s  %s\n", f.Level, f.Area, f.Message)
		}
		if !*connect {
			_, _ = fmt.Fprintln(stdout, "\nThe database and the directory were not contacted; add -connect to check them too.")
		}
	}

	if report.Failed() {
		return errCheckFailed
	}
	return nil
}

// exitCheckConfig runs the subcommand and exits with its status.
func exitCheckConfig(args []string) {
	if err := runCheckConfig(args, os.Stdout); err != nil {
		if err != errCheckFailed {
			_, _ = fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
