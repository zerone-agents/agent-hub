package repository

import (
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/skill"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupSkillRepoTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&skill.Skill{}, &agent.AgentConfig{}, &agent.AgentSkill{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// issue #123：反查按租户切分——own 只含请求租户名单（409 载荷），
// foreign 仅记他租户绑定事实，绝不携带他租户身份。
func TestSkillRepository_GetSkillBindingsScoped_TenantSplit(t *testing.T) {
	db := setupSkillRepoTestDB(t)
	repo := NewSkillRepositoryWithDB(db)

	alpha := &agent.AgentConfig{Name: "alpha", TenantID: "org-a"}
	beta := &agent.AgentConfig{Name: "beta", TenantID: "org-b"}
	require.NoError(t, db.Create(alpha).Error)
	require.NoError(t, db.Create(beta).Error)
	sk := &skill.Skill{Name: "s1", TenantID: "org-a"}
	require.NoError(t, db.Create(sk).Error)
	require.NoError(t, db.Create(&agent.AgentSkill{AgentID: alpha.ID, SkillID: sk.ID}).Error)
	require.NoError(t, db.Create(&agent.AgentSkill{AgentID: beta.ID, SkillID: sk.ID}).Error)

	own, foreign, err := repo.GetSkillBindingsScoped("org-a", sk.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha"}, own)
	require.True(t, foreign)

	// 未绑定 → 全空
	sk2 := &skill.Skill{Name: "s2", TenantID: "org-a"}
	require.NoError(t, db.Create(sk2).Error)
	own2, foreign2, err := repo.GetSkillBindingsScoped("org-a", sk2.ID)
	require.NoError(t, err)
	require.Empty(t, own2)
	require.False(t, foreign2)
}

func TestSkillRepository_GetSkillBindingsScoped_SortedOwn(t *testing.T) {
	db := setupSkillRepoTestDB(t)
	repo := NewSkillRepositoryWithDB(db)
	sk := &skill.Skill{Name: "s1", TenantID: "org-a"}
	require.NoError(t, db.Create(sk).Error)
	for _, n := range []string{"zeta", "alpha", "mid"} {
		a := &agent.AgentConfig{Name: n, TenantID: "org-a"}
		require.NoError(t, db.Create(a).Error)
		require.NoError(t, db.Create(&agent.AgentSkill{AgentID: a.ID, SkillID: sk.ID}).Error)
	}
	own, foreign, err := repo.GetSkillBindingsScoped("org-a", sk.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"alpha", "mid", "zeta"}, own)
	require.False(t, foreign)
}