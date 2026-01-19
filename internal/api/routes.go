package api

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/ldapwarden/ldapwarden/internal/audit"
	"github.com/ldapwarden/ldapwarden/internal/auth"
	"github.com/ldapwarden/ldapwarden/internal/config"
	"github.com/ldapwarden/ldapwarden/internal/ldap"
	"github.com/ldapwarden/ldapwarden/internal/mail"
	"github.com/ldapwarden/ldapwarden/internal/passwordreset"
	"github.com/ldapwarden/ldapwarden/internal/rbac"
	"github.com/ldapwarden/ldapwarden/internal/scheduler"
)

type Server struct {
	router        chi.Router
	ldapClient    *ldap.Client
	authService   *auth.AuthService
	auditLogger   *audit.Logger
	rbac          *rbac.RBAC
	config        *config.Config
	mailer        *mail.Mailer
	passwordReset *passwordreset.Service
	scheduler     *scheduler.Scheduler
}

func NewServer(
	ldapClient *ldap.Client,
	authService *auth.AuthService,
	auditLogger *audit.Logger,
	rbacService *rbac.RBAC,
	cfg *config.Config,
	mailer *mail.Mailer,
	passwordResetService *passwordreset.Service,
	sched *scheduler.Scheduler,
) *Server {
	s := &Server{
		ldapClient:    ldapClient,
		authService:   authService,
		auditLogger:   auditLogger,
		rbac:          rbacService,
		config:        cfg,
		mailer:        mailer,
		passwordReset: passwordResetService,
		scheduler:     sched,
	}

	s.router = s.setupRoutes()
	return s
}

func (s *Server) setupRoutes() chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)

	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"http://localhost:5173", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	r.Route("/api", func(r chi.Router) {
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", s.handleLogin)

			r.Group(func(r chi.Router) {
				r.Use(s.authMiddleware)
				r.Post("/logout", s.handleLogout)
				r.Get("/me", s.handleGetMe)
				r.Post("/change-password", s.handleChangeMyPassword)
			})
		})

		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)

			r.Route("/users", func(r chi.Router) {
				r.With(s.requirePermission(rbac.PermUsersRead)).Get("/", s.handleListUsers)
				r.With(s.requirePermission(rbac.PermUsersRead)).Get("/{dn}", s.handleGetUser)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Post("/", s.handleCreateUser)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Put("/{dn}", s.handleUpdateUser)
				r.With(s.requirePermission(rbac.PermUsersDelete)).Delete("/{dn}", s.handleDeleteUser)
				r.With(s.requirePermission(rbac.PermUsersRead)).Get("/{dn}/groups", s.handleGetUserGroups)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Post("/{dn}/lock", s.handleLockUser)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Post("/{dn}/unlock", s.handleUnlockUser)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Put("/{dn}/expiration", s.handleSetUserExpiration)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Post("/{dn}/password", s.handleChangePassword)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Put("/{dn}/ssh-keys", s.handleSetSSHKeys)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Post("/{dn}/ssh-keys", s.handleAddSSHKey)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Delete("/{dn}/ssh-keys", s.handleRemoveSSHKey)
				r.With(s.requirePermission(rbac.PermUsersRead)).Get("/{dn}/sudo-roles", s.handleGetUserSudoRoles)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Post("/{dn}/send-password-reset", s.handleSendPasswordReset)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Put("/{dn}/samba", s.handleUpdateUserSamba)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Put("/{dn}/shadow", s.handleUpdateUserShadow)
			})

			r.Route("/sudo-roles", func(r chi.Router) {
				r.With(s.requirePermission(rbac.PermUsersRead)).Get("/", s.handleListSudoRoles)
				r.With(s.requirePermission(rbac.PermUsersRead)).Get("/{dn}", s.handleGetSudoRole)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Post("/", s.handleCreateSudoRole)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Put("/{dn}", s.handleUpdateSudoRole)
				r.With(s.requirePermission(rbac.PermUsersDelete)).Delete("/{dn}", s.handleDeleteSudoRole)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Post("/{dn}/users", s.handleAddUserToSudoRole)
				r.With(s.requirePermission(rbac.PermUsersWrite)).Delete("/{dn}/users", s.handleRemoveUserFromSudoRole)
				r.With(s.requirePermission(rbac.PermGroupsWrite)).Post("/{dn}/groups", s.handleAddGroupToSudoRole)
				r.With(s.requirePermission(rbac.PermGroupsWrite)).Delete("/{dn}/groups", s.handleRemoveGroupFromSudoRole)
			})

			r.Route("/password-policies", func(r chi.Router) {
				r.With(s.requirePermission(rbac.PermSettingsRead)).Get("/", s.handleListPasswordPolicies)
				r.With(s.requirePermission(rbac.PermSettingsRead)).Get("/{dn}", s.handleGetPasswordPolicy)
				r.With(s.requirePermission(rbac.PermSettingsWrite)).Post("/", s.handleCreatePasswordPolicy)
				r.With(s.requirePermission(rbac.PermSettingsWrite)).Put("/{dn}", s.handleUpdatePasswordPolicy)
				r.With(s.requirePermission(rbac.PermSettingsWrite)).Delete("/{dn}", s.handleDeletePasswordPolicy)
			})

			r.Route("/groups", func(r chi.Router) {
				r.With(s.requirePermission(rbac.PermGroupsRead)).Get("/", s.handleListGroups)
				r.With(s.requirePermission(rbac.PermGroupsRead)).Get("/{dn}", s.handleGetGroup)
				r.With(s.requirePermission(rbac.PermGroupsWrite)).Post("/", s.handleCreateGroup)
				r.With(s.requirePermission(rbac.PermGroupsWrite)).Put("/{dn}", s.handleUpdateGroup)
				r.With(s.requirePermission(rbac.PermGroupsDelete)).Delete("/{dn}", s.handleDeleteGroup)
				r.With(s.requirePermission(rbac.PermGroupsWrite)).Post("/{dn}/members", s.handleAddGroupMember)
				r.With(s.requirePermission(rbac.PermGroupsWrite)).Delete("/{dn}/members", s.handleRemoveGroupMember)
				r.With(s.requirePermission(rbac.PermGroupsRead)).Get("/{dn}/sudo-roles", s.handleGetGroupSudoRoles)
				r.With(s.requirePermission(rbac.PermGroupsWrite)).Put("/{dn}/samba", s.handleUpdateGroupSamba)
			})

			r.Route("/schema", func(r chi.Router) {
				r.With(s.requirePermission(rbac.PermSchemaRead)).Get("/", s.handleGetSchema)
				r.With(s.requirePermission(rbac.PermSchemaWrite)).Post("/refresh", s.handleRefreshSchema)
			})

			r.Route("/audit-logs", func(r chi.Router) {
				r.With(s.requirePermission(rbac.PermAuditRead)).Get("/", s.handleListAuditLogs)
			})

			r.Route("/next-ids", func(r chi.Router) {
				r.With(s.requirePermission(rbac.PermUsersWrite)).Get("/", s.handleGetNextIDs)
			})

			r.Route("/admin", func(r chi.Router) {
				r.With(s.requirePermission(rbac.PermSettingsRead)).Get("/config", s.handleGetConfig)

				// Scheduled tasks management
				r.Route("/scheduled-tasks", func(r chi.Router) {
					r.With(s.requirePermission(rbac.PermSettingsRead)).Get("/config", s.handleGetScheduledTasksConfig)
					r.With(s.requirePermission(rbac.PermSettingsRead)).Get("/runs", s.handleGetTaskRuns)
					r.With(s.requirePermission(rbac.PermSettingsWrite)).Post("/users-expiration/trigger", s.handleTriggerAccountExpirationTask)
					r.With(s.requirePermission(rbac.PermSettingsWrite)).Post("/passwords-expiration/trigger", s.handleTriggerPasswordExpirationTask)
				})
			})
		})

		// Public password reset endpoints (no auth required)
		r.Route("/password-reset", func(r chi.Router) {
			r.Get("/{token}", s.handleGetPasswordResetInfo)
			r.Post("/{token}", s.handleConfirmPasswordReset)
		})
	})

	// Serve static files from web/dist if it exists
	staticPath := "web/dist"
	if _, err := os.Stat(staticPath); err == nil {
		r.Get("/*", spaHandler(staticPath))
	}

	return r
}

// spaHandler serves static files and falls back to index.html for SPA routing
func spaHandler(staticPath string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(staticPath))

	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Check if the file exists
		fullPath := filepath.Join(staticPath, path)
		if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
			// File exists, serve it
			fileServer.ServeHTTP(w, r)
			return
		}

		// Check if path has a file extension (likely a missing asset)
		if strings.Contains(filepath.Base(path), ".") {
			http.NotFound(w, r)
			return
		}

		// Serve index.html for SPA routing
		http.ServeFile(w, r, filepath.Join(staticPath, "index.html"))
	}
}

// spaFileSystem wraps http.FileSystem to handle SPA routing
type spaFileSystem struct {
	fs fs.FS
}

func (s spaFileSystem) Open(name string) (http.File, error) {
	f, err := s.fs.Open(name)
	if err != nil {
		return nil, err
	}
	return f.(http.File), nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}
