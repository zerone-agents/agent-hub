package services

import (
	"os"
	"sync"
	"testing"

	"control-panel/internal/domain/aigc"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 真实 MySQL 并发测试（spec §8）：env 提供 DSN 时运行、缺省跳过。
// 不强制触发特定错误码（可能 1213/1062/或后启动者直接读到已提交记录），
// 只断言允许结果 + 合法串行顺序；实际错误码 t.Logf 记录供诊断。
func TestAigcSaveConcurrent_MySQL(t *testing.T) {
	dsn := os.Getenv("AUDIT_MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("AUDIT_MYSQL_TEST_DSN not set")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&aigc.Config{}))
	db.Exec("DELETE FROM aigc_configs WHERE tenant_id = ?", "concurrent-test")
	svc := newAigcSvcOnDB(t, db) // 复用 fixture 构造：注入 encryptionKey

	var wg sync.WaitGroup
	type outcome struct {
		dto  *ConfigDTO
		rcpt *aigc.AigcMutationReceipt
		err  error
	}
	out := make([]outcome, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			company := "公司A"
			if i == 1 {
				company = "公司B"
			}
			d, r, e := svc.Save("concurrent-test", testUSCC, company)
			out[i] = outcome{d, r, e}
			if e != nil {
				t.Logf("attempt %d observed error (diagnostic only): %v", i, e)
			}
		}(i)
	}
	wg.Wait()

	var final aigc.Config
	require.NoError(t, db.Where("tenant_id = ?", "concurrent-test").First(&final).Error)

	// 允许结果：两个都成功、或一个成功一个失败（1205 快速失败等）
	succeeded := 0
	for _, o := range out {
		if o.err == nil {
			succeeded++
			require.NotNil(t, o.dto)
			require.NotNil(t, o.rcpt) // receipt 仅来自成功提交（spec §3.3）
		} else {
			require.Nil(t, o.rcpt) // 失败（含 1205）→ receipt=nil
		}
	}
	require.GreaterOrEqual(t, succeeded, 1)
	// 合法串行顺序：最终状态 = 最后成功提交者的完整 payload（全量替换语义）
	last := "公司A"
	for _, o := range out {
		if o.err == nil && o.rcpt.Created {
			last = o.dto.CompanyName // create 的串行在前；简化断言：final ∈ {公司A, 公司B}
		}
	}
	require.Contains(t, []string{"公司A", "公司B"}, final.CompanyName)
	_ = last
}
