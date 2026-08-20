package database

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"control-panel/internal/config"
	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/aigc"
	authdomain "control-panel/internal/domain/auth"
	"control-panel/internal/domain/chat"
	"control-panel/internal/domain/mcp"
	"control-panel/internal/domain/provider"
	"control-panel/internal/domain/scene"
	"control-panel/internal/domain/skill"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDatabase(cfg *config.DatabaseConfig) error {
	var err error

	DB, err = gorm.Open(mysql.Open(cfg.URL), &gorm.Config{
		// Warn: only slow SQL and errors. Info would log every statement with
		// full parameter values, leaking chat message contents into stdout.
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	sqlDB, err := DB.DB()
	if err != nil {
		return fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxIdleConns(cfg.MaxIdle)
	sqlDB.SetMaxOpenConns(cfg.MaxOpen)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.MaxLifetime) * time.Second)

	log.Println("Database connected successfully")
	return nil
}

func GetDB() *gorm.DB {
	return DB
}

func Ping() error {
	sqlDB, err := DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Ping()
}

func Close() error {
	if DB != nil {
		sqlDB, err := DB.DB()
		if err != nil {
			return err
		}
		return sqlDB.Close()
	}
	return nil
}

func AutoMigrate() error {
	if DB == nil {
		return fmt.Errorf("database not initialized")
	}

	if err := migrateProviderAttributeIndex(); err != nil {
		return fmt.Errorf("failed to migrate provider attribute index: %w", err)
	}

	err := DB.AutoMigrate(
		&agent.AgentConfig{},
		&agent.AgentSubagent{},
		&agent.AgentKnowledgeDataset{},
		&agent.Tool{},
		&agent.AgentTool{},
		&agent.AgentSkill{},
		&mcp.McpServer{},
		&mcp.AgentMcpServer{},
		&scene.Scene{},
		&skill.Skill{},
		&chat.Session{},
		&chat.Message{},
		&aigc.Config{},
		&provider.LegacyProvider{},
		&provider.ProviderSummary{},
		&provider.ProviderAttribute{},
		&authdomain.CLIToken{},
		&authdomain.User{},
		&authdomain.Invite{},
		&authdomain.RefreshToken{},
		&authdomain.UserIdentity{},
	)
	if err != nil {
		return fmt.Errorf("failed to auto migrate: %w", err)
	}

	if err := migrateProviderSplit(); err != nil {
		return fmt.Errorf("failed to migrate provider split: %w", err)
	}

	if err := migrateProviderModels(); err != nil {
		return fmt.Errorf("failed to migrate provider models: %w", err)
	}

	if err := migrateProviderOcrEndpointsBackfill(); err != nil {
		return fmt.Errorf("failed to backfill OCR endpoint models: %w", err)
	}

	if err := migrateDropLegacyProviderColumns(); err != nil {
		return fmt.Errorf("failed to drop legacy provider columns: %w", err)
	}

	if err := migrateDropLegacyTenantDomain(); err != nil {
		return fmt.Errorf("failed to drop legacy tenant domain: %w", err)
	}

	if err := migrateDropToolRequiredColumn(); err != nil {
		return fmt.Errorf("failed to drop tool required column: %w", err)
	}

	if err := migrateAgentPlatformFlags(); err != nil {
		return fmt.Errorf("failed to migrate agent platform flags: %w", err)
	}

	if err := migrateAigcModelCodes(DB); err != nil {
		return fmt.Errorf("failed to migrate aigc model codes: %w", err)
	}

	if err := migrateSessionIndexes(); err != nil {
		return fmt.Errorf("failed to migrate session indexes: %w", err)
	}

	log.Println("Database migration completed successfully")
	return nil
}

// migrateProviderSplit copies rows from the legacy vendor_presets backup
// table into the new provider_summaries table — but ONLY when the new
// table is empty (idempotent). The legacy table is never written to or
// dropped by this process; it remains as a safety backup.
//
// TODO: once provider_summaries / provider_attributes are confirmed
// stable in production, DROP TABLE vendor_presets on the next release.
func migrateProviderSplit() error {
	var summaryCount int64
	if err := DB.Model(&provider.ProviderSummary{}).Count(&summaryCount).Error; err != nil {
		return err
	}
	if summaryCount > 0 {
		// Already migrated; never touch the legacy table again.
		return nil
	}

	var legacyCount int64
	if err := DB.Model(&provider.LegacyProvider{}).Count(&legacyCount).Error; err != nil {
		return err
	}
	if legacyCount == 0 {
		// Nothing to migrate.
		return nil
	}

	log.Printf("Migrating %d legacy vendor_presets rows into provider_summaries...", legacyCount)

	// Raw column copy of descriptive fields only. default_models is
	// intentionally excluded: provider_summaries no longer has that
	// column (Task 7 moved models to the provider_models table), so
	// including it would break the INSERT on stale upgrades where
	// vendor_presets is populated but provider_summaries is empty.
	// Trade-off: stale upgraders lose custom models from the legacy
	// JSON blob; SeedIfEmpty in provider_service.go reseeds defaults.
	if err := DB.Exec(`
		INSERT INTO provider_summaries
			(id, ` + "`key`" + `, name, description, description_en, protocol, auth_style,
			 base_url, fields, icon_key, builtin, locked_api_key,
			 created_at, updated_at)
		SELECT
			 id, ` + "`key`" + `, name, description, description_en, protocol, auth_style,
			 base_url, fields, icon_key, builtin, locked_api_key,
			 created_at, updated_at
		FROM vendor_presets
	`).Error; err != nil {
		return fmt.Errorf("copy vendor_presets -> provider_summaries failed: %w", err)
	}

	log.Println("Provider split migration completed")
	return nil
}

// migrateDropLegacyProviderColumns drops the default_models and type columns
// from provider_summaries. These were replaced by the provider_models table
// (model_type column) in earlier migrations. The drop MUST run after
// migrateProviderModels() so the JSON backfill can still read default_models.
// Idempotent: checks HasColumn before dropping.
//
// Uses raw SQL ALTER TABLE DROP COLUMN rather than GORM's
// migrator.DropColumn because the latter silently no-ops on SQLite when the
// field is absent from the struct (and rebuilds via the struct schema, which
// skips fields not on the struct). Raw SQL works uniformly on MySQL and
// modern SQLite (>= 3.35, which glebarez/sqlite bundles). Backticks quote
// the reserved word `type` on both engines.
func migrateDropLegacyProviderColumns() error {
	if DB == nil {
		return nil
	}
	migrator := DB.Migrator()
	if migrator.HasColumn(&provider.ProviderSummary{}, "default_models") {
		if err := DB.Exec("ALTER TABLE provider_summaries DROP COLUMN `default_models`").Error; err != nil {
			return fmt.Errorf("drop default_models failed: %w", err)
		}
		log.Println("Dropped legacy column provider_summaries.default_models")
	}
	if migrator.HasColumn(&provider.ProviderSummary{}, "type") {
		if err := DB.Exec("ALTER TABLE provider_summaries DROP COLUMN `type`").Error; err != nil {
			return fmt.Errorf("drop type failed: %w", err)
		}
		log.Println("Dropped legacy column provider_summaries.type")
	}
	return nil
}

// migrateDropLegacyTenantDomain removes the pre-Casdoor self-built
// multi-tenancy leftovers: the dead tenants / service_deployments /
// resources tables, plus the stray tenant_id + casdoor_user_id columns that
// the legacy tenant.User model leaked into the shared users table via
// AutoMigrate. Casdoor 模式下组织管理不接管，租户标识直接走 JWT org 名
// （user_identities.tenant_id），这套遗留结构无任何消费方。
// Idempotent: HasTable/HasColumn guards; raw SQL for the same
// SQLite-vs-MySQL uniformity reasons as migrateDropLegacyProviderColumns.
func migrateDropLegacyTenantDomain() error {
	if DB == nil {
		return nil
	}
	migrator := DB.Migrator()
	for _, table := range []string{"service_deployments", "resources", "tenants"} {
		if migrator.HasTable(table) {
			if err := DB.Exec(fmt.Sprintf("DROP TABLE `%s`", table)).Error; err != nil {
				return fmt.Errorf("drop %s failed: %w", table, err)
			}
			log.Printf("Dropped legacy table %s", table)
		}
	}
	// users 表被 legacy tenant.User 混入的游离列（auth.User 才是 users 表的
	// owner，不认识这两列）。builtin 模式下 users 表仍承载登录，只清列不删表。
	for _, col := range []string{"tenant_id", "casdoor_user_id"} {
		if migrator.HasColumn("users", col) {
			if err := DB.Exec(fmt.Sprintf("ALTER TABLE users DROP COLUMN `%s`", col)).Error; err != nil {
				return fmt.Errorf("drop users.%s failed: %w", col, err)
			}
			log.Printf("Dropped legacy column users.%s", col)
		}
	}
	return nil
}

// migrateAgentPlatformFlags backfills agents.desktop_enabled from the legacy
// `enabled` column and then drops that column. The new desktop_enabled /
// mobile_enabled columns themselves are created by AutoMigrate from the model
// tags. Idempotent: a no-op once the legacy column is gone.
func migrateAgentPlatformFlags() error {
	var colCount int
	if err := DB.Raw(
		"SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'agents' AND COLUMN_NAME = 'enabled'",
	).Scan(&colCount).Error; err != nil {
		return err
	}
	if colCount == 0 {
		return nil
	}
	log.Println("Backfilling agents.desktop_enabled from legacy enabled column...")
	if err := DB.Exec("UPDATE agents SET desktop_enabled = enabled").Error; err != nil {
		return fmt.Errorf("failed to backfill desktop_enabled: %w", err)
	}
	if err := DB.Exec("ALTER TABLE agents DROP COLUMN enabled").Error; err != nil {
		return fmt.Errorf("failed to drop legacy enabled column: %w", err)
	}
	return nil
}

// migrateProviderAttributeIndex fixes the uk_provider_attr index.
// Originally it was a single-column unique index on (attr_key) alone, but the
// correct constraint is a composite unique index on (provider_id, attr_key).
// GORM AutoMigrate cannot drop/modify existing indexes, so we do it here:
// 1. Drop the old single-column uk_provider_attr index if it exists.
// 2. Delete orphan rows (provider deleted but attributes remained).
// AutoMigrate then recreates uk_provider_attr as a composite index per the
// updated GORM tags in model.go.
func migrateProviderAttributeIndex() error {
	var tableExists bool
	if err := DB.Raw(
		"SELECT COUNT(*) > 0 FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'provider_attributes'",
	).Scan(&tableExists).Error; err != nil {
		return err
	}
	if !tableExists {
		return nil
	}

	// Check if the old single-column index exists (attr_key alone).
	var idxCount int64
	if err := DB.Raw(
		"SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'provider_attributes' AND INDEX_NAME = 'uk_provider_attr'",
	).Scan(&idxCount).Error; err != nil {
		return err
	}
	if idxCount > 0 {
		log.Println("Dropping old single-column uk_provider_attr index from provider_attributes...")
		if err := DB.Exec("DROP INDEX uk_provider_attr ON provider_attributes").Error; err != nil {
			return fmt.Errorf("failed to drop old uk_provider_attr index: %w", err)
		}
		log.Println("Old index dropped; AutoMigrate will recreate as composite (provider_id, attr_key)")
	}

	// Delete orphan rows where provider_id references a deleted provider.
	var orphanCount int64
	if err := DB.Raw(
		"SELECT COUNT(*) FROM provider_attributes pa LEFT JOIN provider_summaries ps ON pa.provider_id = ps.id WHERE ps.id IS NULL",
	).Scan(&orphanCount).Error; err != nil {
		return err
	}
	if orphanCount > 0 {
		log.Printf("Cleaning up %d orphan provider_attributes rows...", orphanCount)
		if err := DB.Exec(
			"DELETE pa FROM provider_attributes pa LEFT JOIN provider_summaries ps ON pa.provider_id = ps.id WHERE ps.id IS NULL",
		).Error; err != nil {
			return fmt.Errorf("failed to clean up orphan attributes: %w", err)
		}
		log.Println("Orphan attributes cleaned up")
	}

	return nil
}

func migrateDropToolRequiredColumn() error {
	var count int64
	if err := DB.Model(&agent.Tool{}).Where("1=1").Limit(1).Count(&count).Error; err != nil {
		return err
	}

	hasColumn := false
	if err := DB.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'tools' AND COLUMN_NAME = 'required'").Scan(&hasColumn).Error; err != nil {
		return err
	}

	if hasColumn {
		log.Println("Dropping 'required' column from 'tools' table...")
		if err := DB.Exec("ALTER TABLE tools DROP COLUMN required").Error; err != nil {
			return err
		}
		log.Println("'required' column dropped successfully")
	}

	return nil
}

// legacyProviderSummary mirrors the provider_summaries row shape as it
// existed BEFORE Task 7 — including the now-dropped default_models JSON
// blob and type column. It is used only by migrateProviderModels() to
// backfill provider_models from the legacy data on databases that still
// have those columns. Reading via this struct lets the production
// ProviderSummary shed the legacy fields.
type legacyProviderSummary struct {
	ID            uint64 `gorm:"column:id"`
	Key           string `gorm:"column:key"`
	Protocol      string `gorm:"column:protocol"`
	Type          string `gorm:"column:type"`
	DefaultModels string `gorm:"column:default_models"`
}

// migrateProviderModels creates the provider_models table and backfills it
// from the legacy default_models JSON column. Idempotent: a no-op if the
// table already has rows. model_type per row = JSON's modelType if present,
// otherwise the provider's type column, otherwise derived from protocol.
//
// Reads via legacyProviderSummary so this works on databases that still
// have the legacy columns (they are dropped by migrateDropLegacyProviderColumns
// immediately after this run).
func migrateProviderModels() error {
	if DB == nil {
		return nil
	}

	// AutoMigrate is safe to call repeatedly.
	if err := DB.AutoMigrate(&provider.ProviderModel{}); err != nil {
		return fmt.Errorf("auto-migrate provider_models failed: %w", err)
	}

	// Skip if already populated (idempotency).
	var existingCount int64
	if err := DB.Model(&provider.ProviderModel{}).Count(&existingCount).Error; err != nil {
		return fmt.Errorf("count provider_models failed: %w", err)
	}
	if existingCount > 0 {
		return nil
	}

	// If the legacy default_models column is already gone (fresh installs
	// post-Task-7, or schemas built from the new struct), there is nothing
	// to backfill from.
	if !DB.Migrator().HasColumn(&provider.ProviderSummary{}, "default_models") {
		return nil
	}

	// The legacy `type` column may already be gone on some databases;
	// include it only when present. When absent, the model-type fallback
	// falls through to protocol-based derivation below.
	selectCols := "id, `key`, protocol, default_models"
	if DB.Migrator().HasColumn(&provider.ProviderSummary{}, "type") {
		selectCols = "id, `key`, protocol, type, default_models"
	}

	var summaries []legacyProviderSummary
	if err := DB.Table("provider_summaries").
		Select(selectCols).
		Find(&summaries).Error; err != nil {
		return fmt.Errorf("load provider_summaries failed: %w", err)
	}

	for _, s := range summaries {
		if s.DefaultModels == "" || s.DefaultModels == "[]" {
			continue
		}
		var models []provider.CatalogModel
		if err := json.Unmarshal([]byte(s.DefaultModels), &models); err != nil {
			log.Printf("migrateProviderModels: skip provider %s (unmarshal error: %v)", s.Key, err)
			continue
		}
		for i, m := range models {
			mt := m.ModelType
			if mt == "" {
				mt = s.Type
			}
			if mt == "" {
				// Inline derivation (kept in sync with the old TypeForProtocol).
				switch provider.Protocol(s.Protocol) {
				case provider.ProtocolMinerU, provider.ProtocolPaddleOCR:
					mt = string(provider.TypeOCR)
				default:
					mt = string(provider.TypeLLM)
				}
			}
			selectionID := m.SelectionID
			if selectionID == "" {
				selectionID = m.ModelID
			}
			row := provider.ProviderModel{
				ProviderID:    s.ID,
				SelectionID:   selectionID,
				ModelID:       m.ModelID,
				DisplayName:   m.DisplayName,
				ModelType:     mt,
				ContextWindow: m.ContextWindow,
				Status:        "1",
				SortOrder:     i,
			}
			if err := DB.Create(&row).Error; err != nil {
				log.Printf("migrateProviderModels: insert failed for %s/%s: %v", s.Key, selectionID, err)
			}
		}
	}

	log.Printf("migrateProviderModels: backfill complete (%d providers scanned)", len(summaries))
	return nil
}

// migrateProviderOcrEndpointsBackfill ensures endpoint-only OCR providers
// (MinerU, PaddleOCR) have at least one row in provider_models. These
// providers historically carried an empty defaultModels slice because the
// endpoint itself IS the service — there was no per-model catalog. After
// Task 7 moved filtering to provider_models.model_type, an empty slice
// meant these providers stopped appearing under ?type=ocr and lost their
// capability chip in the UI.
//
// This migration inserts a single "default" OCR model for any provider
// with protocol mineru/paddleocr that currently has zero model rows.
// Idempotent: once the row exists, subsequent runs are no-op.
func migrateProviderOcrEndpointsBackfill() error {
	if DB == nil {
		return nil
	}

	var endpoints []provider.ProviderSummary
	if err := DB.Where("protocol IN ?", []string{
		string(provider.ProtocolMinerU),
		string(provider.ProtocolPaddleOCR),
	}).Find(&endpoints).Error; err != nil {
		return fmt.Errorf("load OCR endpoint providers failed: %w", err)
	}

	for _, ep := range endpoints {
		var modelCount int64
		if err := DB.Model(&provider.ProviderModel{}).
			Where("provider_id = ?", ep.ID).
			Count(&modelCount).Error; err != nil {
			return fmt.Errorf("count models for %s failed: %w", ep.Key, err)
		}
		if modelCount > 0 {
			continue
		}

		row := provider.ProviderModel{
			ProviderID:  ep.ID,
			SelectionID: ep.Key,
			ModelID:     ep.Key,
			DisplayName: ep.Name,
			ModelType:   string(provider.TypeOCR),
			Status:      "1",
			SortOrder:   0,
		}
		if err := DB.Create(&row).Error; err != nil {
			log.Printf("migrateProviderOcrEndpointsBackfill: insert failed for %s: %v", ep.Key, err)
			continue
		}
		log.Printf("migrateProviderOcrEndpointsBackfill: inserted default OCR model for %s", ep.Key)
	}
	return nil
}

// migrateAigcModelCodes is a one-time migration: backfills the new
// provider_models.aigc_code column from the legacy aigc_configs.model_codes
// JSON blob (preserving codes already deployed to production), assigns
// sequential codes to any remaining rows, then DROPs the legacy column.
//
// Idempotent: if provider_models already has any non-empty aigc_code, the
// backfill is skipped (still attempts the column drop, which is itself a
// no-op if the column is already gone).
//
// GORM's struct-based HasColumn/DropColumn(&aigc.Config{}, "ModelCodes")
// cannot be used here because Task 5 removed ModelCodes from the struct.
// HasColumn accepts table+column strings, but DropColumn on glebarez/sqlite
// rebuilds the table via the struct schema and panics when handed a bare
// table name with no model. We use raw SQL ALTER TABLE DROP COLUMN, matching
// the approach in migrateDropLegacyProviderColumns (works uniformly on MySQL
// and modern SQLite >= 3.35, which glebarez/sqlite bundles).
func migrateAigcModelCodes(db *gorm.DB) error {
	// Idempotency: skip backfill if any model row already has a code.
	var alreadyAssigned int64
	db.Table("provider_models").Where("aigc_code <> ''").Count(&alreadyAssigned)
	if alreadyAssigned == 0 {
		if err := backfillAigcModelCodes(db); err != nil {
			return fmt.Errorf("backfill aigc model codes: %w", err)
		}
	}
	if db.Migrator().HasColumn("aigc_configs", "model_codes") {
		if err := db.Exec("ALTER TABLE aigc_configs DROP COLUMN model_codes").Error; err != nil {
			return fmt.Errorf("drop aigc_configs.model_codes column: %w", err)
		}
	}
	return nil
}

// backfillAigcModelCodes reads the legacy aigc_configs.model_codes JSON blob
// (keyed by ModelID) and writes the matched code onto the corresponding
// provider_models rows. Rows whose ModelID is not in the blob get the next
// sequentially-available 4-digit code (counting up from the max code seen
// so far, blob- or pass-assigned). When the same ModelID appears across
// multiple providers, all rows share the code assigned on its first sighting.
func backfillAigcModelCodes(db *gorm.DB) error {
	// Load legacy blob if the column still exists.
	blobCodes := map[string]string{}
	if db.Migrator().HasColumn("aigc_configs", "model_codes") {
		var blob string
		row := db.Table("aigc_configs").Select("model_codes").Where("id = 1").Row()
		if err := row.Scan(&blob); err == nil && blob != "" {
			_ = json.Unmarshal([]byte(blob), &blobCodes)
		}
	}

	var rows []provider.ProviderModel
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
		return err
	}
	assigned := make(map[string]string, len(rows))
	max := 0
	for _, r := range rows {
		if r.ModelID == "" {
			continue
		}
		if code, ok := assigned[r.ModelID]; ok {
			// already assigned earlier in this pass (cross-row same ModelID)
			if err := db.Model(&provider.ProviderModel{}).Where("id = ?", r.ID).Update("aigc_code", code).Error; err != nil {
				return err
			}
			continue
		}
		var code string
		if c, ok := blobCodes[r.ModelID]; ok {
			code = c
		} else {
			max++
			code = fmt.Sprintf("%04d", max)
		}
		assigned[r.ModelID] = code
		if n, err := strconv.Atoi(code); err == nil && n > max {
			max = n
		}
		if err := db.Model(&provider.ProviderModel{}).Where("id = ?", r.ID).Update("aigc_code", code).Error; err != nil {
			return err
		}
	}
	return nil
}

// migrateSessionIndexes adds the updated_at index to cloud_sessions if missing.
// Without this index, ORDER BY updated_at DESC triggers a full-table in-memory
// sort that can exhaust MySQL's sort_buffer_size on large tables.
func migrateSessionIndexes() error {
	if DB == nil {
		return nil
	}
	if DB.Migrator().HasIndex(&chat.Session{}, "UpdatedAt") {
		return nil
	}
	log.Println("Adding updated_at index to cloud_sessions...")
	if err := DB.Migrator().CreateIndex(&chat.Session{}, "UpdatedAt"); err != nil {
		return fmt.Errorf("create updated_at index on cloud_sessions: %w", err)
	}
	log.Println("cloud_sessions updated_at index created")
	return nil
}
