package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"floorball-score-table/internal/config"
	"floorball-score-table/internal/db"
	apphttp "floorball-score-table/internal/http"
	"floorball-score-table/internal/match"
	"floorball-score-table/internal/views"
)

func main() {
	var (
		addr       = flag.String("addr", envOr("ADDR", ":8080"), "listen address")
		configPath = flag.String("config", envOr("CONFIG_PATH", "/config/teams.yaml"), "teams config path (yaml)")
		dbPath     = flag.String("db", envOr("DB_PATH", "/data/app.db"), "sqlite db path")
	)
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	sqlDB, err := db.OpenAndInit(*dbPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer sqlDB.Close()

	renderer, err := views.NewRenderer(os.DirFS("web/templates"))
	if err != nil {
		log.Fatalf("templates: %v", err)
	}

	active := match.NewActiveMatchStore()
	edit := match.NewEditStore()
	hub := apphttp.NewScoreHub()

	h := apphttp.NewHandlers(apphttp.HandlersDeps{
		Config:   cfg,
		DB:       sqlDB,
		Renderer: renderer,
		Active:   active,
		Edit:     edit,
		Hub:      hub,
	})

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RedirectSlashes)
	r.Use(middleware.Logger)

	r.Mount("/static", http.StripPrefix("/static", http.FileServer(http.Dir("web/static"))))

	r.Get("/display_score", h.DisplayScore)
	r.Get("/events/score", h.ScoreEvents)

	// Support both /control_panel and /control_panel/ (people often omit the trailing slash).
	r.Get("/control_panel", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/control_panel/", http.StatusMovedPermanently)
	})

	r.Route("/control_panel", func(r chi.Router) {
		r.Get("/", h.ControlPanel)
		r.Get("/teams", h.TeamsOverview)
		r.Post("/start_match", h.StartMatch)

		r.Get("/active_match", h.ActiveMatch)
		r.Post("/active_match/player/{playerID}/inc", h.PlayerInc)
		r.Post("/active_match/player/{playerID}/dec", h.PlayerDec)
		r.Post("/active_match/save", h.SaveMatch)
		r.Post("/active_match/discard", h.DiscardMatch)
		r.Post("/active_match/discard_beacon", h.DiscardMatchBeacon)

		r.Get("/history", h.History)
		r.Get("/history/{matchID}/edit", h.EditMatch)
		r.Post("/history/{matchID}/edit/player/{playerID}/inc", h.EditPlayerInc)
		r.Post("/history/{matchID}/edit/player/{playerID}/dec", h.EditPlayerDec)
		r.Post("/history/{matchID}/edit/save", h.SaveMatchEdits)
		r.Post("/history/{matchID}/edit/discard", h.DiscardMatchEdits)
		r.Post("/history/{matchID}/delete", h.DeleteMatch)

		r.Get("/stats", h.Stats)
	})

	srv := &http.Server{
		Addr:              *addr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	waitForShutdown(func(ctx context.Context) error {
		log.Printf("shutting down")
		return srv.Shutdown(ctx)
	})
}

func waitForShutdown(shutdown func(ctx context.Context) error) {
	stop := make(chan os.Signal, 2)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := shutdown(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "shutdown: %v\n", err)
		os.Exit(1)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
