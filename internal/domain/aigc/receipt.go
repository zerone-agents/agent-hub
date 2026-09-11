package aigc

// AigcMutationReceipt：Save 的权威变更回执（spec §3.3）。
// 仅在最终成功提交的尝试上生成；失败（含 1205 快速失败）调用方收到 nil。
type AigcMutationReceipt struct {
	Created       bool
	ChangedFields []AigcConfigField // create=全集；update=实际值变化字段；幂等=[]
}
