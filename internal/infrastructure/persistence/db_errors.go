package repository

import (
	"errors"
	"strings"

	"github.com/go-sql-driver/mysql"
)

// IsFKConstraintError 判断删除是否被外键约束（ON DELETE RESTRICT，#123
// review P1 并发后盾）拒绝。生产 MySQL：errno 1451 = ER_ROW_IS_REFERENCED_2
// （行仍被引用，RESTRICT 拒绝删除）；sqlite 测试：错误消息含
// "FOREIGN KEY constraint failed"。service 层据此把约束冲突映射为
// InUse 错误（409），而不是暴露 500。
func IsFKConstraintError(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1451
	}
	return strings.Contains(err.Error(), "FOREIGN KEY constraint failed")
}
