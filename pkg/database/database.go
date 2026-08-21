package database

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"strings"
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

// defaultBackfillTenant 是回填存量数据 tenant_id 时的兜底租户。
// 本地常量而非 import internal/domain/tenant，避免 pkg → internal 的循环依赖。
const defaultBackfillTenant = "default"

// 包级变量，由 AutoMigrate 设置、各 migrate 回填函数消费
var backfillTenantID = defaultBackfillTenant

// AutoMigrate runs the automatic migration for all models.
// backfillTenant 指定存量行空 tenant_id 的回填目标；传空时回落到 defaultBackfillTenant。
func AutoMigrate(backfillTenant string) error {
	if backfillTenant == "" {
		backfillTenant = defaultBackfillTenant
	}
	backfillTenantID = backfillTenant
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

	if err := migrateChatTenantID(); err != nil {
		return fmt.Errorf("failed to migrate chat tenant id: %w", err)
	}

	if err := migrateAgentsTenantID(); err != nil {
		return fmt.Errorf("failed to migrate agents tenant id: %w", err)
	}

	if err := migrateProvidersTenantID(); err != nil {
		return fmt.Errorf("failed to migrate providers tenant id: %w", err)
	}

	if err := migrateMcpToolsSkillsScenesTenantID(); err != nil {
		return fmt.Errorf("failed to migrate mcp/tools/skills/scenes tenant id: %w", err)
	}

	if err := migrateDropLegacyUkTenantName(); err != nil {
		return fmt.Errorf("failed to drop legacy uk_tenant_name indexes: %w", err)
	}

	if err := migrateAigcConfigsTenantID(); err != nil {
		return fmt.Errorf("failed to migrate aigc_configs tenant id: %w", err)
	}

	if err := migrateDropVendorPresets(); err != nil {
		return fmt.Errorf("failed to drop vendor_presets: %w", err)
	}

	log.Println("Database migration completed successfully")
	return nil
}

// migrateProviderSplit copies rows from the legacy vendor_presets backup
// table into the new provider_summaries table — but ONLY when the new
// table is empty (idempotent). The legacy table is dropped for good by
// migrateDropVendorPresets at the end of the chain; on databases where it
// is already gone (fresh installs) this is a no-op.
func migrateProviderSplit() error {
	var summaryCount int64
	if err := DB.Model(&provider.ProviderSummary{}).Count(&summaryCount).Error; err != nil {
		return err
	}
	if summaryCount > 0 {
		// Already migrated; never touch the legacy table again.
		return nil
	}

	if !DB.Migrator().HasTable("vendor_presets") {
		// Fresh database or already dropped — nothing to salvage.
		return nil
	}

	var legacyCount int64
	if err := DB.Raw("SELECT COUNT(*) FROM `vendor_presets`").Scan(&legacyCount).Error; err != nil {
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

// migrateDropVendorPresets drops the legacy vendor_presets backup table for
// good — provider_summaries / provider_attributes have been the source of
// truth long enough to be considered stable, and multi-tenant Phase 3 made
// the legacy single-tenant table moot anyway. Runs last in the chain so
// migrateProviderSplit still gets a chance to salvage rows from stale
// databases first. Idempotent: HasTable guard, no-op when already dropped.
func migrateDropVendorPresets() error {
	if DB == nil {
		return nil
	}
	migrator := DB.Migrator()
	if migrator.HasTable("vendor_presets") {
		if err := DB.Exec("DROP TABLE `vendor_presets`").Error; err != nil {
			return fmt.Errorf("drop vendor_presets failed: %w", err)
		}
		log.Println("Dropped legacy table vendor_presets")
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
	// information_schema 是 MySQL 专有；sqlite（迁移测试）直接跳过——本函数
	// 只处理遗留 enabled 列的回填/删除，不影响加列与租户回填。
	if DB.Dialector.Name() != "mysql" {
		return nil
	}
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
	// information_schema 是 MySQL 专有；sqlite（迁移测试）直接跳过——
	// 本函数只做索引修正/孤儿行清理，不影响加列与数据回填。
	if DB.Dialector.Name() != "mysql" {
		return nil
	}
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

	// 用 Migrator().HasColumn 而非 information_schema——后者是 MySQL 专有，
	// sqlite（迁移测试）没有该表。
	hasColumn := DB.Migrator().HasColumn("tools", "required")

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

// migrateChatTenantID backfills tenant_id on cloud_sessions / cloud_messages.
// The column itself is added by AutoMigrate from the model tags.
func migrateChatTenantID() error {
	if DB == nil {
		return nil
	}
	// 加列由 AutoMigrate 完成；这里回填：
	// 1) 能经 user_id → user_identities.tenant_id 映射的按映射回填
	// 2) 映射不到的（历史脏数据）回填启动租户
	for _, table := range []string{"cloud_sessions", "cloud_messages"} {
		if !DB.Migrator().HasColumn(table, "tenant_id") {
			continue // 防御：列还没加（理论不会发生，AutoMigrate 已跑）
		}
		// MySQL/SQLite 通用 UPDATE...WHERE IN (SELECT)
		if err := DB.Exec(fmt.Sprintf(
			"UPDATE `%s` SET tenant_id = (SELECT tenant_id FROM user_identities WHERE user_identities.external_id = `%s`.user_id LIMIT 1) WHERE user_id IN (SELECT external_id FROM user_identities)",
			table, table)).Error; err != nil {
			return fmt.Errorf("backfill %s from user_identities: %w", table, err)
		}
		if err := BackfillTenantID(DB, table); err != nil {
			return err
		}
	}
	return nil
}

// migrateAgentsTenantID 回填 agents 存量行的 tenant_id（启动租户），并把旧的
// 全局唯一索引 uk_name 换成复合索引 uk_tenant_name (tenant_id, name)——
// 加列和建新索引由 AutoMigrate 完成，这里只负责回填 + 删旧索引。
func migrateAgentsTenantID() error {
	if DB == nil {
		return nil
	}
	m := DB.Migrator()
	if err := BackfillTenantID(DB, "agents"); err != nil {
		return err
	}
	if m.HasIndex(&agent.AgentConfig{}, "uk_name") {
		if err := m.DropIndex(&agent.AgentConfig{}, "uk_name"); err != nil {
			return fmt.Errorf("drop agents.uk_name: %w", err)
		}
		log.Println("Dropped agents.uk_name (replaced by uk_tenant_name)")
	}
	return nil
}

// migrateProvidersTenantID 回填 provider_summaries 存量行的 tenant_id（启动
// 租户），并把旧的全局唯一索引 uk_key 换成复合索引 uk_tenant_key
// (tenant_id, key)——加列和建新索引由 AutoMigrate 完成，这里只负责回填 +
// 删旧索引。provider_attributes / provider_models 子表不加 tenant 列，
// 归属校验经主表。注意：本次回填之后播种的种子 provider 走共享语义
// （tenant_id=”），回填只发生在加列后的这一次。
func migrateProvidersTenantID() error {
	if DB == nil {
		return nil
	}
	m := DB.Migrator()
	if err := BackfillTenantID(DB, "provider_summaries"); err != nil {
		return err
	}
	if m.HasIndex(&provider.ProviderSummary{}, "uk_key") {
		if err := m.DropIndex(&provider.ProviderSummary{}, "uk_key"); err != nil {
			return fmt.Errorf("drop provider_summaries.uk_key: %w", err)
		}
		log.Println("Dropped provider_summaries.uk_key (replaced by uk_tenant_key)")
	}
	return nil
}

// migrateMcpToolsSkillsScenesTenantID 回填 mcp_servers / tools / skills /
// scenes 四表存量行的 tenant_id（启动租户），并把旧的全局唯一索引 uk_name
// 换成复合索引 uk_tenant_name (tenant_id, name)——加列和建新索引由
// AutoMigrate 完成，这里只负责回填 + 删旧索引。
//
// 内置行处理（决策 1A）：tools/mcps 的内置行（is_builtin=true，及
// tools SeedIfEmpty 的预设行）是全平台模板，回填后统一改回共享语义
// tenant_id=”；scenes 有 agent_id 归属列，优先按 agents 主表回填，
// 映射不到的再走启动租户兜底。关联表（agent_tools/agent_skills/
// agent_mcp_servers）不加 tenant 列，归属校验经主表。
func migrateMcpToolsSkillsScenesTenantID() error {
	if DB == nil {
		return nil
	}
	m := DB.Migrator()

	legacyModels := map[string]interface{}{
		"mcp_servers": &mcp.McpServer{},
		"tools":       &agent.Tool{},
		"skills":      &skill.Skill{},
		"scenes":      &scene.Scene{},
	}

	// 幂等守卫：回填 + 归零 + 删旧索引只在真正执行迁移的那一次跑——判据
	// 是旧全局唯一索引 uk_name 仍存在（AutoMigrate 已建 uk_tenant_name，
	// 本函数删掉 uk_name 后重跑即为纯 no-op）。归零 UPDATE 绝不能每次启动
	// 都跑：租户可与共享预设行同名（('org-a','Skill') 与 ('','Skill') 合法
	// 共存），无条件按名单归零会把租户私有行劫持进共享域，甚至撞
	// uk_tenant_name 唯一索引导致启动失败。
	needsMigration := false
	for _, model := range legacyModels {
		if m.HasIndex(model, "uk_name") {
			needsMigration = true
			break
		}
	}

	if needsMigration {
		// scenes：先按 agents 主表回填（同 migrateChatTenantID 的 JOIN 回填）
		if m.HasColumn("scenes", "tenant_id") {
			if err := DB.Exec(
				"UPDATE `scenes` SET tenant_id = (SELECT tenant_id FROM `agents` WHERE `agents`.id = `scenes`.agent_id LIMIT 1) WHERE agent_id IN (SELECT id FROM `agents`)",
			).Error; err != nil {
				return fmt.Errorf("backfill scenes from agents: %w", err)
			}
		}

		for _, table := range []string{"mcp_servers", "tools", "skills", "scenes"} {
			if err := BackfillTenantID(DB, table); err != nil {
				return err
			}
		}

		// 回填后，内置行改为共享（''）：这些是全平台模板，不属于任何租户。
		// 注：tools 表没有 is_builtin 列（预设行名单见下一条 UPDATE）。
		if err := DB.Exec("UPDATE `mcp_servers` SET tenant_id = '' WHERE is_builtin = ?", true).Error; err != nil {
			return fmt.Errorf("reset builtin mcp_servers to shared: %w", err)
		}
		// tools 的 SeedIfEmpty/SeedBuiltins 预设行没有 is_builtin 标志，按
		// agent.PresetToolNames 固定名单一并归入共享模板——名单与 seeding
		// 同源维护，新增预设工具两处自动同步。
		placeholders := make([]string, len(agent.PresetToolNames))
		args := make([]interface{}, len(agent.PresetToolNames))
		for i, name := range agent.PresetToolNames {
			placeholders[i] = "?"
			args[i] = name
		}
		if err := DB.Exec(
			fmt.Sprintf("UPDATE `tools` SET tenant_id = '' WHERE name IN (%s)", strings.Join(placeholders, ",")),
			args...,
		).Error; err != nil {
			return fmt.Errorf("reset seeded tools to shared: %w", err)
		}
	}

	for table, model := range legacyModels {
		if m.HasIndex(model, "uk_name") {
			if err := m.DropIndex(model, "uk_name"); err != nil {
				return fmt.Errorf("drop %s.uk_name: %w", table, err)
			}
			log.Printf("Dropped %s.uk_name (replaced by uk_tenant_name)", table)
		}
	}
	return nil
}

// migrateDropLegacyUkTenantName 清理索引改名残留：复合唯一索引原名
// uk_tenant_name 在 agents/tools/skills/mcp_servers/scenes 五表共用（MySQL
// 索引名 table-scoped 合法，SQLite 全局唯一会撞名，导致真实 AutoMigrate
// 无法在 sqlite 测试库上运行），已改为每表唯一名（uk_agents_tenant_name
// 等）。旧库升级后 AutoMigrate 会建好新名索引，这里删掉旧名残留；新库
// 无旧名索引，纯 no-op，天然幂等。
func migrateDropLegacyUkTenantName() error {
	if DB == nil {
		return nil
	}
	m := DB.Migrator()
	for table, model := range map[string]interface{}{
		"agents":      &agent.AgentConfig{},
		"tools":       &agent.Tool{},
		"skills":      &skill.Skill{},
		"mcp_servers": &mcp.McpServer{},
		"scenes":      &scene.Scene{},
	} {
		if m.HasIndex(model, "uk_tenant_name") {
			if err := m.DropIndex(model, "uk_tenant_name"); err != nil {
				return fmt.Errorf("drop %s.uk_tenant_name: %w", table, err)
			}
			log.Printf("Dropped %s.uk_tenant_name (replaced by per-table unique index)", table)
		}
	}
	return nil
}

// migrateAigcConfigsTenantID 把 aigc_configs 从"id 恒为 1"的单行全局配置
// 迁到 per-tenant + 共享回退。存量行的 tenant_id 置 ”（共享默认），保持
// 升级前"全局一份"的行为逐字节等价：任何租户没有自己的行时都读到同一份。
//
// 一次性置 ” 的幂等保证：列由 AutoMigrate 以 default:” 加出（对齐
// Task 4/5 的共享哨兵约定），MySQL/SQLite 的 ADD COLUMN ... NOT NULL
// DEFAULT ” 语义本身就一次性地把全部存量行填为 ”，且后续任何代码路径
// 都不会再写出 ” 以外的默认值或 NULL。本函数因此只兜底归一异常中间态
// （手工改库、中断的半迁移）产生的 NULL/不存在值——归一条件永不再命中，
// 天然幂等。不能按 id=1 或 tenant_id='default' 之类条件 UPDATE：新库中
// id=1 可能已是某租户自己的行，条件 UPDATE 会把租户行误改成共享行。
func migrateAigcConfigsTenantID() error {
	if DB == nil {
		return nil
	}
	m := DB.Migrator()
	if !m.HasTable("aigc_configs") || !m.HasColumn("aigc_configs", "tenant_id") {
		return nil
	}
	if err := DB.Exec("UPDATE aigc_configs SET tenant_id = '' WHERE tenant_id IS NULL").Error; err != nil {
		return fmt.Errorf("normalize aigc_configs tenant_id: %w", err)
	}
	return nil
}

// BackfillTenantID 把存量行的空 tenant_id 回填为启动租户（幂等：只动空值）。
// "空即存量"的前提是各 model 的 tenant_id 列以 default:” 加出——AutoMigrate
// 的 ADD COLUMN ... NOT NULL DEFAULT ” 会把全部存量行一次性填为 ”，因此
// WHERE tenant_id = ” OR tenant_id IS NULL 恰好命中且仅命中存量行；共享行
// 的空串语义在回填之后才由代码引入，不会与回填条件冲突。
func BackfillTenantID(db *gorm.DB, table string) error {
	res := db.Exec(fmt.Sprintf("UPDATE `%s` SET tenant_id = ? WHERE tenant_id = '' OR tenant_id IS NULL", table), backfillTenantID)
	if res.Error != nil {
		return fmt.Errorf("backfill %s.tenant_id: %w", table, res.Error)
	}
	if res.RowsAffected > 0 {
		log.Printf("Backfilled %d rows in %s to tenant %q", res.RowsAffected, table, backfillTenantID)
	}
	return nil
}
