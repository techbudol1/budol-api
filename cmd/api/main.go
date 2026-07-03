package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"budol/server/internal/config"
	"budol/server/internal/gmrengine"
	"budol/server/internal/httpapi"
	"budol/server/internal/newsagent"
	"budol/server/internal/session"
	"budol/server/internal/store"
	"budol/server/internal/thirdweb"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	userStore, err := store.NewMemgraphUserStore(ctx, cfg.MemgraphURI, cfg.MemgraphUser, cfg.MemgraphPassword)
	if err != nil {
		log.Fatalf("memgraph connection error: %v", err)
	}
	defer userStore.Close(context.Background())

	seedCtx, seedCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer seedCancel()
	if err := userStore.SeedDefaults(seedCtx, cfg.SeedDemoPolls); err != nil {
		log.Fatalf("default data seed error: %v", err)
	}

	newsScout, err := newsagent.New(context.Background(), cfg, userStore)
	if err != nil {
		log.Fatalf("news agent setup error: %v", err)
	}
	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	go newsScout.Start(workerCtx)

	app := httpapi.New(
		cfg,
		userStore,
		thirdweb.NewClient(cfg.ThirdwebMeURL, cfg.ThirdwebSendURL, cfg.ThirdwebSecretKey),
		gmrengine.NewClient(cfg.GMREngineAPIBase, cfg.GMREngineAPIKey),
		newsScout,
		session.NewManager(cfg.SessionSecret, cfg.SessionTTL),
	)

	errs := make(chan error, 1)
	go func() {
		errs <- app.Listen(cfg.Addr)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errs:
		log.Fatalf("server error: %v", err)
	case <-quit:
		stopWorkers()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := app.ShutdownWithContext(shutdownCtx); err != nil {
			log.Printf("server shutdown error: %v", err)
		}
	}
}
