package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/mthidayat/dompet-cerdas-go/internal/config"
	"github.com/mthidayat/dompet-cerdas-go/internal/middleware"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/account"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/advisor"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/health"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/reminder"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/telegram"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/telegram/botapi"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/db"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/gemini"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/ratelimit"
)

func main() {
	// 1. Load Config
	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 2. Setup Logger
	var handler slog.Handler
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// 3. Initialize Firebase/Firestore
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	firebaseApp, err := db.NewFirebase(ctx, cfg.FirebaseProjectID, cfg.GoogleCredentials)
	if err != nil {
		slog.Error("Failed to initialize Firebase", "error", err)
		os.Exit(1)
	}
	defer firebaseApp.Close()

	// 4. Initialize Core Utilities
	rateLimiter := ratelimit.NewMemoryLimiter()
	defer rateLimiter.Stop()

	// 5. Initialize Services & Handlers
	accountService := account.NewService(firebaseApp.Firestore)
	accountRepository := account.NewRepository(firebaseApp.Firestore, accountService)
	accountHandler := account.NewHandler(accountService, accountRepository)

	quotaManager := advisor.NewQuotaManager(firebaseApp.Firestore)

	// The classifier is optional: without a Gemini key the bot still resolves
	// categories deterministically and simply asks for confirmation more often,
	// rather than refusing to record transactions.
	var categoryClassifier transaction.Classifier
	var transactionParser transaction.TextParser
	var receiptAnalyzer transaction.ReceiptAnalyzer
	var insightGenerator advisor.InsightGenerator
	var voiceTranscriber telegram.Transcriber
	if geminiClient, err := gemini.NewClient(ctx, cfg.GeminiAPIKey); err != nil {
		slog.Warn("Gemini unavailable, category classification and receipt scanning will degrade", "error", err)
	} else {
		categoryClassifier = geminiClient
		transactionParser = geminiClient
		receiptAnalyzer = geminiClient
		insightGenerator = geminiClient
		voiceTranscriber = geminiClient
	}

	advisorService := advisor.NewService(accountService, accountRepository, insightGenerator, quotaManager)
	advisorHandler := advisor.NewHandler(advisorService)

	transactionHandler := transaction.NewHandler(receiptAnalyzer)

	telegramHandler := telegram.NewHandler(
		firebaseApp.Firestore,
		accountService,
		accountRepository,
		categoryClassifier,
		transactionParser,
		receiptAnalyzer,
		advisorService,
		voiceTranscriber,
		cfg.TelegramBotToken,
		cfg.TelegramWebhookSecret,
	)
	healthHandler := health.NewHandler(firebaseApp, cfg.Env)

	// 6. Initialize Cron Scheduler
	// Generating a simple instance ID (e.g. hostname or random) for the leader lock
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown-host"
	}
	instanceID := fmt.Sprintf("%s-%d", hostname, os.Getpid())

	cronManager := reminder.NewCronManager(firebaseApp.Firestore, instanceID, botapi.New(cfg.TelegramBotToken), accountService)
	if err := cronManager.Start(); err != nil {
		slog.Error("Failed to start cron scheduler", "error", err)
		os.Exit(1)
	}
	defer cronManager.Stop()

	// 7. Setup HTTP Router
	router := gin.New()
	router.Use(middleware.Recovery())
	router.Use(middleware.Logger())
	router.Use(middleware.CORS(cfg.CORSAllowedOrigins))

	router.NoRoute(middleware.NoRoute())
	router.NoMethod(middleware.NoMethod())

	// API v1 Routing
	v1 := router.Group("/api/v1")

	// Public endpoints. The Telegram webhook cannot sit behind Firebase auth
	// because Telegram has no Firebase identity; it is guarded by
	// TELEGRAM_WEBHOOK_SECRET instead.
	healthHandler.Register(v1)
	telegramHandler.RegisterPublic(v1)

	// Protected endpoints (Firebase ID token required)
	protected := v1.Group("")
	protected.Use(middleware.Auth(firebaseApp.Auth))
	{
		accountHandler.Register(protected)
		advisorHandler.Register(protected)
		transactionHandler.Register(protected)
		telegramHandler.RegisterProtected(protected)
	}

	// 8. Start Server with Graceful Shutdown
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", cfg.Port),
		Handler: router,
	}

	go func() {
		slog.Info("Server starting", "port", cfg.Port, "env", cfg.Env)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("Listen failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	slog.Info("Shutting down server...")

	ctxShutDown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctxShutDown); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	slog.Info("Server exiting")
}
