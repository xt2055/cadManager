package app

import (
	"context"
	"log"
	"net/http"

	"cadguanliq/internal/auth"
	"cadguanliq/internal/config"
	"cadguanliq/internal/data"
	httpapi "cadguanliq/internal/http"
	"github.com/jackc/pgx/v5/pgxpool"
)

func NewHandler(cfg config.Config, pool *pgxpool.Pool, authService *auth.Service) http.Handler {
	return httpapi.NewRouter(cfg, pool, authService)
}

func Run(cfg config.Config) error {
	pool, err := data.NewPool(context.Background(), cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()
	authService := auth.NewService(auth.NewPGRepository(pool))
	if err := authService.MigrateLegacyUsers(context.Background()); err != nil {
		return err
	}

	log.Printf("cadguanliq backend listening on %s", cfg.Addr)
	return http.ListenAndServe(cfg.Addr, NewHandler(cfg, pool, authService))
}
