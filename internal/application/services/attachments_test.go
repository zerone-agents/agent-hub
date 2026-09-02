package services

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateUploadsPath(t *testing.T) {
	require.NoError(t, ValidateUploadsPath(".zerone-uploads/report.pdf"))
	require.Error(t, ValidateUploadsPath("report.pdf"))                       // 无前缀
	require.Error(t, ValidateUploadsPath(".zerone-uploads/"))                 // 空文件名
	require.Error(t, ValidateUploadsPath(".zerone-uploads/a/b.txt"))          // 嵌套
	require.Error(t, ValidateUploadsPath(".zerone-uploads/../secret"))        // 逃逸
	require.Error(t, ValidateUploadsPath(".zerone-uploads/.."))               // 逃逸
	require.Error(t, ValidateUploadsPath("/abs/.zerone-uploads/a"))           // 绝对路径
	require.Error(t, ValidateUploadsPath(".zerone-uploads/a\x00b"))           // NUL
	require.Error(t, ValidateUploadsPath("data:application/pdf;base64,AAAA")) // base64 伪装
}

func validDesc() AttachmentDesc {
	return AttachmentDesc{ID: "f-1", Name: "report.pdf", Mime: "application/pdf", Size: 12, Path: ".zerone-uploads/report.pdf"}
}

func TestValidateAttachmentDescs(t *testing.T) {
	require.NoError(t, ValidateAttachmentDescs(nil))
	require.NoError(t, ValidateAttachmentDescs([]AttachmentDesc{validDesc()}))

	tooMany := make([]AttachmentDesc, 11)
	for i := range tooMany {
		tooMany[i] = validDesc()
	}
	err := ValidateAttachmentDescs(tooMany)
	require.Error(t, err)
	require.Contains(t, err.Error(), "10")

	bad := validDesc()
	bad.Path = "/etc/passwd"
	require.Error(t, ValidateAttachmentDescs([]AttachmentDesc{bad}))

	badSize := validDesc()
	badSize.Size = 21 << 20
	require.Error(t, ValidateAttachmentDescs([]AttachmentDesc{badSize}))

	badName := validDesc()
	badName.Name = ""
	require.Error(t, ValidateAttachmentDescs([]AttachmentDesc{badName}))
}
