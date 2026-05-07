package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/ldapwarden/ldapwarden/internal/api"
	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/auth"
	"github.com/ldapwarden/ldapwarden/internal/config"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
	"github.com/ldapwarden/ldapwarden/internal/mail"
	"github.com/ldapwarden/ldapwarden/internal/passwordreset"
	"github.com/ldapwarden/ldapwarden/internal/rbac"
	"github.com/ldapwarden/ldapwarden/internal/scheduler"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := config.Load()

	log.Printf("Connecting to PostgreSQL at %s", cfg.Database.URL)
	pool, err := pgxpool.New(ctx, cfg.Database.URL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}
	log.Println("Connected to PostgreSQL")

	// Run database migrations
	if err := runMigrations(cfg.Database.URL); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}

	log.Printf("Connecting to Redis at %s", cfg.Redis.URL)
	redisOpts, err := redis.ParseURL(cfg.Redis.URL)
	if err != nil {
		return fmt.Errorf("parse redis URL: %w", err)
	}
	redisClient := redis.NewClient(redisOpts)
	defer func() { _ = redisClient.Close() }()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("ping redis: %w", err)
	}
	log.Println("Connected to Redis")

	log.Printf("Connecting to LDAP at %s", cfg.LDAP.URL)
	ldapClient := ldap.NewClient(cfg.LDAP)
	defer ldapClient.Close()
	log.Println("LDAP client initialized")

	sessionStore := auth.NewRedisSessionStore(redisClient)
	authService := auth.NewAuthService(ldapClient, sessionStore, cfg.Session.TTL, cfg.App.AdminGroup)
	rbacService := rbac.NewRBAC(cfg.App.AdminGroup)
	mailer := mail.NewMailer(&cfg.Mail, cfg.App.Organization)
	auditLogger := audit.NewLogger(pool, mailer, cfg.App.AuditNotifyEmails)
	passwordResetService := passwordreset.NewService(pool)

	// Initialize scheduler for background tasks
	sched := scheduler.New(cfg, ldapClient, mailer, pool, auditLogger)
	if err := sched.Start(ctx); err != nil {
		log.Printf("Warning: failed to start scheduler: %v", err)
	}

	server := api.NewServer(ldapClient, authService, auditLogger, rbacService, cfg, mailer, passwordResetService, sched)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      server,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("Server starting on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	// Stop scheduler first
	sched.Stop()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	log.Println("Server stopped")
	return nil
}

func runMigrations(databaseURL string) error {
	d, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("create migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", d, databaseURL)
	if err != nil {
		return fmt.Errorf("create migrate instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("run migrations: %w", err)
	}

	version, dirty, _ := m.Version()
	if dirty {
		log.Printf("Warning: database migration is in dirty state at version %d", version)
	} else {
		log.Printf("Database migrations up to date (version %d)", version)
	}

	return nil
}
