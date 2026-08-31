package services

import (
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"

	"control-panel/internal/domain/agent"
)

// MaxToolFileSize 单文件上限 5 MiB，与 agent-deployer installer 契约一致。
const MaxToolFileSize = 5 * 1024 * 1024

// toolFileExts 大小写敏感（".TS" 拒绝），与 deployer toolFileExts 契约一致。
var toolFileExts = map[string]struct{}{".ts": {}, ".mts": {}, ".js": {}, ".mjs": {}}

// ToolFileInput 承载单文件制品的原始上传三元组（multipart 解析结果）。
// CreateCustomToolInput（内嵌）与 UploadToolFile 共用，单一定义避免两处重复。
type ToolFileInput struct {
	FileName string
	File     io.Reader
	FileSize int64
}

// BuildToolOSSKey 内容寻址 key（issue #88）：tools/<tenant>/<name>/<sha256><ext>。
// 替换文件即换 hash 即换 key，天然避免覆盖冲突。
func BuildToolOSSKey(tenantID, name, hash, ext string) string {
	return fmt.Sprintf("tools/%s/%s/%s%s", tenantID, name, hash, ext)
}

// validateToolFileBytes 校验扩展名、非空、5 MiB 上限，读全量字节并计算
// sha256。返回 (bytes, ext, hashHex)。hash 本地计算而非采信 Upload 返回值：
// key 与 DB 字段必须同源（内容寻址），生产实现两者必然一致。
func validateToolFileBytes(fileName string, size int64, r io.Reader) ([]byte, string, string, error) {
	ext := filepath.Ext(fileName)
	if _, ok := toolFileExts[ext]; !ok {
		return nil, "", "", agent.ErrInvalidToolFile
	}
	if size > MaxToolFileSize {
		return nil, "", "", agent.ErrToolFileTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(r, MaxToolFileSize+1))
	if err != nil {
		return nil, "", "", fmt.Errorf("读取工具文件失败: %w", err)
	}
	if len(data) == 0 {
		return nil, "", "", agent.ErrToolFileEmpty
	}
	if int64(len(data)) > MaxToolFileSize {
		return nil, "", "", agent.ErrToolFileTooLarge
	}
	sum := sha256.Sum256(data)
	return data, ext, fmt.Sprintf("%x", sum), nil
}
