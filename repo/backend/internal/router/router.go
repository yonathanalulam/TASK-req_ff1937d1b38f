package router

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/eagle-point/service-portal/internal/address"
	"github.com/eagle-point/service-portal/internal/audit"
	"github.com/eagle-point/service-portal/internal/auth"
	catalog_pkg "github.com/eagle-point/service-portal/internal/catalog"
	"github.com/eagle-point/service-portal/internal/config"
	"github.com/eagle-point/service-portal/internal/health"
	hmacadmin_pkg "github.com/eagle-point/service-portal/internal/hmacadmin"
	ingest_pkg "github.com/eagle-point/service-portal/internal/ingest"
	lakehouse_pkg "github.com/eagle-point/service-portal/internal/lakehouse"
	"github.com/eagle-point/service-portal/internal/middleware"
	"github.com/eagle-point/service-portal/internal/models"
	moderation_pkg "github.com/eagle-point/service-portal/internal/moderation"
	notification_pkg "github.com/eagle-point/service-portal/internal/notification"
	privacy_pkg "github.com/eagle-point/service-portal/internal/privacy"
	"github.com/eagle-point/service-portal/internal/profile"
	qa_pkg "github.com/eagle-point/service-portal/internal/qa"
	review_pkg "github.com/eagle-point/service-portal/internal/review"
	"github.com/eagle-point/service-portal/internal/session"
	shipping_pkg "github.com/eagle-point/service-portal/internal/shipping"
	ticket_pkg "github.com/eagle-point/service-portal/internal/ticket"
)

// New builds and returns the configured Gin engine.
func New(cfg *config.Config, db *sql.DB) *gin.Engine {
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()

	// ── Global middleware ────────────────────────────────────────────────────
	r.Use(gin.Recovery())
	r.Use(requestLogger())
	r.Use(corsHeaders(cfg))

	// ── Shared middleware instances ──────────────────────────────────────────
	sessionStore := session.New(db)
	authMW := middleware.NewAuth(db, sessionStore)
	csrfMW := middleware.NewCSRF(db)
	generalRL := middleware.NewGeneralLimiter()
	reportRL := middleware.NewReviewReportLimiter()
	// Reviews: 10 submissions/hour per user (spans create + update + report
	// on the same bucket so a determined abuser cannot split the quota).
	reviewRL := middleware.NewReviewReportLimiter()
	hmacVerifier := middleware.NewHMACVerifier(db, cfg.FieldEncryptionKey)

	// ── Services ─────────────────────────────────────────────────────────────
	authSvc := auth.NewService(db, sessionStore)
	authHandler := auth.NewHandler(authSvc, sessionStore, cfg)

	profileSvc := profile.NewService(db, cfg.FieldEncryptionKey)
	profileHandler := profile.NewHandler(profileSvc)

	addressSvc := address.NewService(db, cfg.FieldEncryptionKey)
	addressHandler := address.NewHandler(addressSvc)

	catalogSvc := catalog_pkg.NewService(db)
	catalogHandler := catalog_pkg.NewHandler(catalogSvc, profileSvc)

	shippingSvc := shipping_pkg.NewService(db)
	shippingHandler := shipping_pkg.NewHandler(shippingSvc)

	ticketSvc := ticket_pkg.NewService(db, "storage/uploads")
	ticketHandler := ticket_pkg.NewHandler(ticketSvc)

	reviewSvc := review_pkg.NewService(db, "storage/uploads")
	reviewHandler := review_pkg.NewHandler(reviewSvc)

	qaSvc := qa_pkg.NewService(db)
	qaHandler := qa_pkg.NewHandler(qaSvc)

	notifSvc := notification_pkg.NewService(db)
	notifHandler := notification_pkg.NewHandler(notifSvc)

	modSvc := moderation_pkg.NewService(db)
	modHandler := moderation_pkg.NewHandler(modSvc)

	auditSvc := audit.NewService(db)
	auditHandler := audit.NewHandler(auditSvc)

	privacySvc := privacy_pkg.NewService(db, auditSvc, "storage/exports")
	privacyHandler := privacy_pkg.NewHandler(privacySvc)

	ingestSvc := ingest_pkg.NewService(db, cfg.FieldEncryptionKey)
	ingestHandler := ingest_pkg.NewHandler(ingestSvc)

	lakehouseSvc := lakehouse_pkg.NewService(db, "storage/lakehouse", "storage/backups/lakehouse")
	lakehouseHandler := lakehouse_pkg.NewHandler(lakehouseSvc)
	// Wire the lakehouse writer into the ingest handler so RunJob can persist
	// Bronze/Silver/Gold outputs. Kept as a runtime setter (not a constructor
	// arg) to avoid a circular import between ingest and lakehouse packages.
	ingestHandler.SetWriter(lakehouseSvc)

	hmacAdminSvc := hmacadmin_pkg.NewService(db, cfg.FieldEncryptionKey)
	hmacAdminHandler := hmacadmin_pkg.NewHandler(hmacAdminSvc, auditSvc)

	// Dev-only: auto-provision an HMAC key on first run so the internal API
	// is usable without manual setup. Skipped in test (tests seed their own
	// keys) and production (admins must provision via the API).
	if cfg.AppEnv == "development" {
		if err := hmacadmin_pkg.EnsureDevKey(context.Background(), db, cfg.FieldEncryptionKey); err != nil {
			_ = err // best-effort; do not block startup
		}
	}

	// ── Phase 7+8 wiring: cross-package hooks ────────────────────────────────
	if err := notification_pkg.SeedDefaultTemplates(context.Background(), db); err != nil {
		_ = err
	}
	_ = modSvc.ReloadTerms(context.Background())

	ticketSvc.SetNotifier(func(ctx context.Context, userID uint64, code string, vars map[string]any) error {
		_, err := notifSvc.Dispatch(ctx, userID, code, vars)
		return err
	})

	authSvc.SetLockoutNotifier(func(ctx context.Context, userID uint64, until time.Time) {
		_, _ = notifSvc.Dispatch(ctx, userID, models.NotifAccountLockout, map[string]any{
			"Until": until.Format(time.RFC3339),
		})
		_ = auditSvc.Write(ctx, audit.Entry{
			UserID: &userID, Action: models.AuditActionLockout,
			Metadata: map[string]interface{}{"until": until.Format(time.RFC3339)},
		})
	})

	authHandler.SetUnreadCounter(func(ctx context.Context, userID uint64) (int, error) {
		return notifSvc.UnreadCount(ctx, userID)
	})

	borderlineHook := func(ctx context.Context, contentType string, contentID uint64, text string, terms []string) error {
		return modSvc.OnBorderlineFlagged(ctx, contentType, contentID, text, terms)
	}
	reviewHandler.SetBorderlineHook(borderlineHook)
	qaHandler.SetBorderlineHook(borderlineHook)

	// ── Background workers ───────────────────────────────────────────────────
	slaNotify := func(ctx context.Context, userID uint64, code string, vars map[string]any) error {
		_, err := notifSvc.Dispatch(ctx, userID, code, vars)
		return err
	}
	bgCtx := context.Background()
	go ticket_pkg.StartSLAEngine(bgCtx, db, slaNotify)
	go privacy_pkg.StartExportWorker(bgCtx, privacySvc)
	go privacy_pkg.StartDeletionWorker(bgCtx, privacySvc)
	// Lakehouse lifecycle runs daily — archive ≥90 days of bronze, purge
	// archived data older than 18 months. Active legal holds skip both ops.
	go lakehouse_pkg.StartLifecycleWorker(bgCtx, lakehouseSvc,
		lakehouse_pkg.DefaultArchiveDays, lakehouse_pkg.DefaultPurgeDays)

	// ── Health ───────────────────────────────────────────────────────────────
	r.GET("/health", health.Handler(db))

	// ── Public auth routes ───────────────────────────────────────────────────
	authGroup := r.Group("/api/v1/auth")
	authGroup.Use(generalRL.Limit())
	{
		authGroup.POST("/register", authHandler.Register)
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/logout", authMW.RequireAuth(), csrfMW.Validate(), authHandler.Logout)
		authGroup.GET("/me", authMW.RequireAuth(), authHandler.Me)
	}

	// ── Public catalog & shipping reads ──────────────────────────────────────
	public := r.Group("/api/v1")
	public.GET("/service-categories", catalogHandler.ListCategories)
	public.GET("/shipping/regions", shippingHandler.ListRegions)
	public.GET("/shipping/templates", shippingHandler.ListTemplates)
	public.GET("/service-offerings/:id/reviews", reviewHandler.ListByOffering)
	public.GET("/service-offerings/:id/review-summary", reviewHandler.Summary)

	// ── HMAC-protected internal routes (Phase 10 ingestion) ──────────────────
	internal := r.Group("/api/v1/internal", hmacVerifier.ValidateHMAC())
	internal.GET("/data/sources", ingestHandler.ListSources)
	internal.POST("/data/sources", ingestHandler.CreateSource)
	internal.PUT("/data/sources/:id", ingestHandler.UpdateSource)
	internal.GET("/data/jobs", ingestHandler.ListJobs)
	internal.POST("/data/jobs", ingestHandler.CreateJob)
	internal.GET("/data/jobs/:id", ingestHandler.GetJob)
	internal.GET("/data/schema-versions/:source_id", ingestHandler.ListSchemaVersions)
	internal.GET("/data/catalog", lakehouseHandler.ListCatalog)
	internal.GET("/data/catalog/:id", lakehouseHandler.GetCatalog)
	internal.GET("/data/lineage/:id", lakehouseHandler.GetLineage)

	// ── Protected API ────────────────────────────────────────────────────────
	api := r.Group("/api/v1",
		authMW.RequireAuth(),
		csrfMW.Validate(),
		generalRL.Limit(),
	)

	// ── Admin sub-group ──────────────────────────────────────────────────────
	admin := api.Group("/admin", middleware.RequireRole(models.RoleAdministrator))
	admin.GET("/hmac-keys", hmacAdminHandler.List)
	admin.POST("/hmac-keys", hmacAdminHandler.Create)
	admin.POST("/hmac-keys/rotate", hmacAdminHandler.Rotate)
	admin.DELETE("/hmac-keys/:id", hmacAdminHandler.Revoke)
	admin.POST("/service-categories", catalogHandler.CreateCategory)
	admin.PUT("/service-categories/:id", catalogHandler.UpdateCategory)
	admin.DELETE("/service-categories/:id", catalogHandler.DeleteCategory)
	admin.POST("/shipping/regions", shippingHandler.CreateRegion)
	admin.POST("/shipping/templates", shippingHandler.CreateTemplate)
	admin.PUT("/shipping/templates/:id", shippingHandler.UpdateTemplate)
	admin.GET("/notification-templates", notifHandler.AdminListTemplates)
	admin.PUT("/notification-templates/:code", notifHandler.AdminUpsertTemplate)
	admin.GET("/sensitive-terms", modHandler.ListTerms)
	admin.POST("/sensitive-terms", modHandler.AddTerm)
	admin.DELETE("/sensitive-terms/:id", modHandler.DeleteTerm)
	admin.GET("/users/:user_id/violations", modHandler.ListUserViolations)
	// Phase 9: audit log + admin hard-delete
	admin.GET("/audit-logs", auditHandler.AdminList)
	admin.DELETE("/users/:user_id", func(c *gin.Context) {
		// Translate :user_id → :id and reuse the privacy handler.
		c.Params = append(c.Params, gin.Param{Key: "id", Value: c.Param("user_id")})
		privacyHandler.AdminHardDelete(c)
	})
	// Phase 10: legal holds (admin)
	admin.GET("/legal-holds", lakehouseHandler.AdminListHolds)
	admin.POST("/legal-holds", lakehouseHandler.AdminPlaceHold)
	admin.DELETE("/legal-holds/:id", lakehouseHandler.AdminReleaseHold)
	// Admin-triggered lifecycle sweep (archive + purge, legal-hold-aware).
	// The background worker below runs this on a schedule; this endpoint
	// is the manual/on-demand entry point.
	admin.POST("/lakehouse/lifecycle/run", lakehouseHandler.AdminRunLifecycle)

	// ── Data Operator sub-group ──────────────────────────────────────────────
	// Session-authenticated operational surface for ingestion + lakehouse.
	// Accessible to data_operator OR administrator (admin is a superset).
	// These endpoints duplicate a subset of the HMAC-protected /internal
	// endpoints, but with session/CSRF/RBAC instead of HMAC — the two surfaces
	// serve different consumers (human operator via the UI vs machine clients).
	dataops := api.Group("/dataops",
		middleware.RequireRole(models.RoleDataOperator, models.RoleAdministrator),
	)
	dataops.GET("/sources", ingestHandler.ListSources)
	dataops.POST("/sources", ingestHandler.CreateSource)
	dataops.PUT("/sources/:id", ingestHandler.UpdateSource)
	dataops.GET("/jobs", ingestHandler.ListJobs)
	dataops.POST("/jobs", ingestHandler.CreateJob)
	dataops.GET("/jobs/:id", ingestHandler.GetJob)
	dataops.POST("/jobs/:id/run", ingestHandler.RunJob)
	dataops.GET("/schema-versions/:source_id", ingestHandler.ListSchemaVersions)
	dataops.GET("/catalog", lakehouseHandler.ListCatalog)
	dataops.GET("/catalog/:id", lakehouseHandler.GetCatalog)
	dataops.GET("/lineage/:id", lakehouseHandler.GetLineage)

	// ── Phase 3: User profile, preferences, favorites, history, addresses ────
	users := api.Group("/users/me")
	{
		users.GET("/profile", profileHandler.GetProfile)
		users.PUT("/profile", profileHandler.UpdateProfile)
		users.GET("/preferences", profileHandler.GetPreferences)
		users.PUT("/preferences", profileHandler.UpdatePreferences)
		users.GET("/favorites", profileHandler.ListFavorites)
		users.POST("/favorites", profileHandler.AddFavorite)
		users.DELETE("/favorites/:offering_id", profileHandler.RemoveFavorite)
		users.GET("/history", profileHandler.ListHistory)
		users.DELETE("/history", profileHandler.ClearHistory)
		users.GET("/addresses", addressHandler.List)
		users.POST("/addresses", addressHandler.Create)
		users.PUT("/addresses/:id", addressHandler.Update)
		users.DELETE("/addresses/:id", addressHandler.Delete)
		users.PUT("/addresses/:id/default", addressHandler.SetDefault)

		// Phase 7
		users.GET("/notifications", notifHandler.List)
		users.GET("/notifications/unread-count", notifHandler.UnreadCount)
		users.GET("/notifications/outbox", notifHandler.Outbox)
		users.PATCH("/notifications/read-all", notifHandler.MarkAllRead)
		users.PATCH("/notifications/:id/read", notifHandler.MarkRead)

		// Phase 9: privacy center
		users.POST("/export-request", privacyHandler.RequestExport)
		users.GET("/export-request/status", privacyHandler.ExportStatus)
		users.GET("/export-request/download", privacyHandler.Download)
		users.POST("/deletion-request", privacyHandler.RequestDeletion)
		users.GET("/deletion-request/status", privacyHandler.DeletionStatus)
	}

	// ── Phase 4: Service offerings & shipping estimate ────────────────────────
	api.GET("/service-offerings", catalogHandler.ListOfferings)
	api.GET("/service-offerings/:id", catalogHandler.GetOffering)
	api.POST("/service-offerings",
		middleware.RequireRole(models.RoleServiceAgent, models.RoleAdministrator),
		catalogHandler.CreateOffering,
	)
	api.PUT("/service-offerings/:id", catalogHandler.UpdateOffering)
	api.PATCH("/service-offerings/:id/status", catalogHandler.ToggleStatus)
	api.POST("/shipping/estimate", shippingHandler.Estimate)

	// ── Phase 5: Tickets ─────────────────────────────────────────────────────
	api.GET("/tickets", ticketHandler.List)
	api.POST("/tickets", modSvc.FreezeCheck(), ticketHandler.Create)
	api.GET("/tickets/:id", ticketHandler.Get)
	api.PATCH("/tickets/:id/status", ticketHandler.UpdateStatus)
	api.GET("/tickets/:id/notes", ticketHandler.ListNotes)
	api.POST("/tickets/:id/notes",
		modSvc.FreezeCheck(),
		modSvc.ScreenContent("content"),
		ticketHandler.CreateNote,
	)
	api.GET("/tickets/:id/attachments", ticketHandler.ListAttachments)
	api.DELETE("/tickets/:id/attachments/:file_id", ticketHandler.DeleteAttachment)

	// ── Phase 6: Reviews ─────────────────────────────────────────────────────
	// Review create/update/report are each throttled to 10/hour per authenticated
	// user. Each action has its own bucket so a single rate-limit message only
	// blocks the action that tripped the limit.
	api.POST("/tickets/:id/reviews",
		reviewRL.Limit(),
		modSvc.FreezeCheck(),
		modSvc.ScreenContent("text"),
		reviewHandler.Create,
	)
	api.PUT("/tickets/:id/reviews/:review_id",
		reviewRL.Limit(),
		modSvc.FreezeCheck(),
		modSvc.ScreenContent("text"),
		reviewHandler.Update,
	)
	api.POST("/reviews/:id/reports", reportRL.Limit(), reviewHandler.CreateReport)

	// ── Phase 6: Q&A ─────────────────────────────────────────────────────────
	api.GET("/service-offerings/:id/qa", qaHandler.ListThreads)
	api.POST("/service-offerings/:id/qa",
		middleware.RequireRole(models.RoleRegularUser, models.RoleAdministrator),
		modSvc.FreezeCheck(),
		modSvc.ScreenContent("question"),
		qaHandler.CreateThread,
	)
	api.POST("/service-offerings/:id/qa/:thread_id/replies",
		middleware.RequireRole(models.RoleServiceAgent, models.RoleAdministrator),
		modSvc.FreezeCheck(),
		modSvc.ScreenContent("content"),
		qaHandler.CreateReply,
	)
	api.DELETE("/qa/:post_id",
		middleware.RequireRole(models.RoleModerator, models.RoleAdministrator),
		qaHandler.DeletePost,
	)

	// ── Phase 8: Moderation queue & actions ──────────────────────────────────
	moderation := api.Group("/moderation",
		middleware.RequireRole(models.RoleModerator, models.RoleAdministrator),
	)
	moderation.GET("/queue", modHandler.ListQueue)
	moderation.POST("/queue/:id/approve", modHandler.Approve)
	moderation.POST("/queue/:id/reject", modHandler.Reject)
	moderation.GET("/actions", modHandler.ListActions)

	// ── 404 fallback ─────────────────────────────────────────────────────────
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"code":    "not_found",
				"message": "the requested resource does not exist",
			},
		})
	})

	return r
}

func requestLogger() gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/health"},
	})
}

func corsHeaders(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		if origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, X-Key-ID, X-Signature")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	}
}
