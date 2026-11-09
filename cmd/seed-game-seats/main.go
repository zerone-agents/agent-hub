package main

import (
	"log"
	"strings"

	"control-panel/internal/application/services"
	"control-panel/internal/config"
	"control-panel/pkg/database"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if err := database.InitDatabase(&cfg.Database); err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	backfill := "default"
	if !cfg.Auth.IsBuiltin() {
		backfill = strings.TrimSpace(cfg.Casdoor.Organization)
	}
	if err := database.AutoMigrate(backfill); err != nil {
		log.Fatal(err)
	}
	runs := services.NewRunService(database.GetDB())
	agents := services.NewAgentService(cfg.Provider.EncryptionKey, "")
	providers := services.NewProviderService(cfg.Provider.EncryptionKey)
	seats := services.NewGameSeatService(database.GetDB(), "default", runs, agents, providers, 120)
	if err := seats.Migrate(); err != nil {
		log.Fatal(err)
	}
	if err := seats.SeedWolfchaResidents(); err != nil {
		log.Fatal(err)
	}
	log.Println("Seeded six Wolfcha residents")
}
