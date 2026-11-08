package knowledge

// UploadPartialError carries only confirmed identities from a failed upload.
// Clients preserve these IDs and reconcile before retrying unknown files.
type UploadPartialError struct {
	Uploaded []Document
}

func (e *UploadPartialError) Error() string {
	return "部分文档已上传，请保留已返回的文档并刷新核对，不要重新上传整批文件"
}

func (e *UploadPartialError) Unwrap() error { return NewUpstreamError(e.Error(), nil) }

type ChunkImageRecovery struct {
	Outcome   string `json:"outcome"`
	Stage     string `json:"stage"`
	Index     string `json:"index"`
	Image     string `json:"image"`
	ImageID   string `json:"image_id"`
	RetrySafe bool   `json:"retry_safe"`
}

type ChunkImageWriteError struct {
	Recovery ChunkImageRecovery
}

func (e *ChunkImageWriteError) Error() string {
	return "切片图片写入结果未确认，请保留草稿并读取切片和图片核对；不要重发创建或追加，需重写图片时使用 replace"
}

func (e *ChunkImageWriteError) Unwrap() error { return NewUpstreamError(e.Error(), nil) }
