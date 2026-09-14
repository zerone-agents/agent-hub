package aigc

// AigcConfigField 是 AIGC 配置操作者字段闭集（spec §3.3）。
// contentProducer（由 uscc 派生）与签名 Key（由 aigc.rotate_key 事件独立覆盖）不计入。
type AigcConfigField string

const (
	AigcFieldUSCC        AigcConfigField = "uscc"
	AigcFieldCompanyName AigcConfigField = "companyName"
)
