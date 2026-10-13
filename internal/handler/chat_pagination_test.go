package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// parsePagination 是所有聊天分页端点（admin 两条 + agent 聊天两条）的唯一入参解析点，
// 因此两条边界（pageSize 上限 / page 上界）在此锁定：任一条失守都会让 SQL LIMIT 或
// OFFSET 被外部放大（前者把整表拉进内存，后者整型溢出成负 offset → GORM 静默返回首页）。
func TestParsePagination_Bounds(t *testing.T) {
	cases := []struct {
		name         string
		rawQuery     string
		wantPage     int
		wantPageSize int
	}{
		{name: "未传取默认", rawQuery: "", wantPage: 1, wantPageSize: 30},
		{name: "合法值原样透传", rawQuery: "page=2&page_size=50", wantPage: 2, wantPageSize: 50},
		{name: "边界值 100 原样透传", rawQuery: "page_size=100", wantPage: 1, wantPageSize: 100},

		{name: "page_size 超界钳到 100", rawQuery: "page_size=200", wantPage: 1, wantPageSize: 100},
		{name: "page_size 极大值钳到 100", rawQuery: "page_size=100000000", wantPage: 1, wantPageSize: 100},
		{name: "page 超过上界钳到 maxPage", rawQuery: "page=1000001", wantPage: maxPage, wantPageSize: 30},
		// 溢出用例：page=MaxInt64 时 (page-1)*pageSize 会翻成负数 offset
		{name: "page 溢出值钳到 maxPage", rawQuery: "page=9223372036854775807", wantPage: maxPage, wantPageSize: 30},

		// 非法/越下界值沿用「未传」语义（取默认），不 400：存量客户端可能发脏值
		{name: "page=0 取默认", rawQuery: "page=0", wantPage: 1, wantPageSize: 30},
		{name: "page_size 负数取默认", rawQuery: "page_size=-5", wantPage: 1, wantPageSize: 30},
		{name: "非数字取默认", rawQuery: "page=abc&page_size=xyz", wantPage: 1, wantPageSize: 30},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodGet, "/?"+tc.rawQuery, nil)

			page, pageSize := parsePagination(c, 1, 30)

			require.Equal(t, tc.wantPage, page)
			require.Equal(t, tc.wantPageSize, pageSize)
		})
	}
}
