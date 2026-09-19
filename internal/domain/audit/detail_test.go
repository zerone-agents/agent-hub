package audit

import (
	"encoding/json"
	"testing"

	"control-panel/internal/domain/aigc"

	"github.com/stretchr/testify/require"
)

// wire key 必须精确 camelCase（CONTRIBUTING JSON 规范；spec §5.4 快照测试）。
func TestDetailWireKeys(t *testing.T) {
	cases := []struct {
		name   string
		detail any
		want   string
	}{
		{"ChangeDetail", ChangeDetail{Field: "role", From: "member", To: "admin"}, `{"field":"role","from":"member","to":"admin"}`},
		{"LoginDetail", LoginDetail{Username: "u1", Org: "org1", Reason: ReasonInvalidCredentials}, `{"username":"u1","org":"org1","reason":"invalid_credentials"}`},
		{"LoginDetailEmptyReason", LoginDetail{Username: "u1"}, `{"username":"u1","org":"","reason":""}`},
		{"InviteDetail", InviteDetail{Role: "maintainer", ExpiresInDays: 7}, `{"role":"maintainer","expiresInDays":7}`},
		{"AigcConfigDetail", AigcConfigDetail{ChangedFields: []aigc.AigcConfigField{aigc.AigcFieldUSCC}}, `{"changedFields":["uscc"]}`},
		{"AigcConfigDetailEmpty", AigcConfigDetail{ChangedFields: []aigc.AigcConfigField{}}, `{"changedFields":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.detail)
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(b))
		})
	}
}
