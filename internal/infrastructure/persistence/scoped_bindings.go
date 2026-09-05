package repository

import (
	"gorm.io/gorm"
)

// scoped_bindings.go —— 绑定表租户切分反查的共享实现（issue #123 P3：
// 技能/MCP/工具/知识库四个 Get*BindingsScoped 曾各自复制同一套
// 「JOIN agents 按 tenant_id 切分 own/foreign」逻辑。隐私边界规则
// （外租户 Agent 名绝不外泄、own 名单 ASC 稳定序）收敛为本文件的唯一
// 实现，杜绝漂移。

// bindingRow 是绑定表 JOIN agents 的查询行，是租户切分的原子单元。
// ResourceID 仅 dataset 批量反查使用（dataset_id 别名）；单资源反查为空。
type bindingRow struct {
	ResourceID string
	AgentName  string
	TenantID   string
}

// queryBindingRows 查询绑定表的绑定行（JOIN agents，name ASC 稳定序）。
//   - resourceID 非 nil 时按 <idColumn> = resourceID 过滤（单资源反查）；
//   - resourceID 为 nil 时返回全量且 resourceCol 必须提供（批量反查，
//     GetDatasetBindingsScoped 使用，dataset_id 化为 resource_id）。
type queryBindingRowsArgs struct {
	BindingTable string
	IDColumn     string
	ResourceID   any
	ResourceCol  string // 批量反查时的资源标识列（如 dataset_id）
}

func queryBindingRows(db *gorm.DB, args queryBindingRowsArgs) ([]bindingRow, error) {
	sel := "agents.name as agent_name, agents.tenant_id as tenant_id"
	if args.ResourceCol != "" {
		sel = args.BindingTable + "." + args.ResourceCol + " as resource_id, " + sel
	}
	q := db.Table(args.BindingTable).
		Select(sel).
		Joins("JOIN agents ON " + args.BindingTable + ".agent_id = agents.id").
		Order("agents.name ASC")
	if args.ResourceID != nil {
		q = q.Where(args.BindingTable+"."+args.IDColumn+" = ?", args.ResourceID)
	}
	var rows []bindingRow
	err := q.Find(&rows).Error
	return rows, err
}

// splitBindingRows 将查询行按请求租户切分为 own 名单（保持 ASC 序）与
// foreign 中性标记。隐私边界（review P1/P3）：own 之外的租户 Agent 名
// 绝不进入返回值，foreign 仅表达「他租户仍绑定」这一事实。
func splitBindingRows(rows []bindingRow, tenantID string) (own []string, foreign bool) {
	for _, r := range rows {
		if r.TenantID == tenantID {
			own = append(own, r.AgentName)
		} else {
			foreign = true
		}
	}
	return own, foreign
}

// resourceBindingsScoped 单资源删除守卫反查的共享实现：技能/MCP/工具
// 三个 Get*BindingsScoped 均收敛到本函数。
func resourceBindingsScoped(db *gorm.DB, bindingTable, idColumn string, resourceID any, tenantID string) ([]string, bool, error) {
	rows, err := queryBindingRows(db, queryBindingRowsArgs{BindingTable: bindingTable, IDColumn: idColumn, ResourceID: resourceID})
	if err != nil {
		return nil, false, err
	}
	own, foreign := splitBindingRows(rows, tenantID)
	return own, foreign, nil
}
