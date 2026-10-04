package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fahad/dashboard/internal/app"
	"github.com/fahad/dashboard/internal/auth"
	"github.com/fahad/dashboard/internal/config"
	"github.com/fahad/dashboard/internal/db"
)

var version = "dev"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// Handle CLI subcommands before loading full config.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "useradd":
			runUserAdd()
			return
		case "migrate-data":
			runMigrateData()
			return
		}
	}

	cfg, err := config.Load()
	if err != nil {
		slog.Error("loading config", "error", err)
		os.Exit(1)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		slog.Error("opening database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Shutdown context used by background goroutines (session cleanup, auto-purge).
	shutdownCtx, shutdownCancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer shutdownCancel()

	router, err := app.NewRouter(shutdownCtx, cfg, database, version)
	if err != nil {
		slog.Error("building router", "error", err)
		os.Exit(1)
	}

	slog.Info("starting server", "addr", cfg.Addr)
	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// runUserAdd handles the "useradd" CLI subcommand.
func runUserAdd() {
	fs := flag.NewFlagSet("useradd", flag.ExitOnError)
	email := fs.String("email", "", "user email address")
	password := fs.String("password", "", "user password")
	firstName := fs.String("first-name", "", "user first name (optional)")
	_ = fs.Parse(os.Args[2:]) // ExitOnError: Parse exits on failure

	if *email == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "usage: dashboard useradd --email <email> --password <password> [--first-name <name>]")
		os.Exit(1)
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "/data/db/dashboard.db"
	}

	database, err := db.Open(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "opening database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	id, err := auth.CreateUser(database, *email, *firstName, *password)
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating user: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("created user %q with id %d\n", *email, id)
}
