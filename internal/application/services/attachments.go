package services

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// Attachment limits shared by the upload proxy and message validation.
// Kept in lockstep with agent-runtime src/uploads.ts.
const (
	MaxAttachmentsPerMessage = 10
	maxAttachmentIDLen       = 64
	maxAttachmentNameLen     = 255
	maxAttachmentMimeLen     = 127
	MaxAttachmentFileBytes   = 20 << 20

	// MaxAttachmentTotalBytes caps the aggregate payload of a single upload
	// request; the handler's request-body ceiling keeps a separate margin
	// above it for boundary/headers.
	MaxAttachmentTotalBytes int64 = 50 << 20

	uploadsDirPrefix = ".zerone-uploads/"
)

// AttachmentDesc describes a runtime-hosted uploaded file. It is the exact
// wire shape of runtime POST /v1/files/uploads response items and of Run
// request attachments — hub relays it verbatim (never rewrites name/size;
// the runtime re-validates both).
type AttachmentDesc struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Mime string `json:"mime"`
	Size int64  `json:"size"`
	Path string `json:"path"`
}

// ValidateUploadsPath reports whether p is a canonical flat
// `.zerone-uploads/<filename>` relative path: prefix enforced, single
// segment, no separators / dot-dot / control characters, not absolute.
func ValidateUploadsPath(p string) error {
	if !strings.HasPrefix(p, uploadsDirPrefix) {
		return fmt.Errorf("attachment path must start with %q", uploadsDirPrefix)
	}
	name := strings.TrimPrefix(p, uploadsDirPrefix)
	if name == "" || name == "." || name == ".." ||
		strings.ContainsAny(name, "/\\") || strings.Contains(p, "\x00") {
		return errors.New("attachment path must be a single filename under .zerone-uploads")
	}
	for _, r := range p {
		if unicode.IsControl(r) {
			return errors.New("attachment path contains control characters")
		}
	}
	return nil
}

// ValidateAttachmentDesc validates one attachment descriptor from a client.
func ValidateAttachmentDesc(a AttachmentDesc) error {
	if a.ID == "" || len(a.ID) > maxAttachmentIDLen {
		return fmt.Errorf("attachment id must be 1-%d characters", maxAttachmentIDLen)
	}
	if a.Name == "" || len(a.Name) > maxAttachmentNameLen {
		return fmt.Errorf("attachment name must be 1-%d characters", maxAttachmentNameLen)
	}
	if len(a.Mime) > maxAttachmentMimeLen {
		return fmt.Errorf("attachment mime must be at most %d characters", maxAttachmentMimeLen)
	}
	if a.Size < 0 || a.Size > MaxAttachmentFileBytes {
		return errors.New("attachment size out of range")
	}
	return ValidateUploadsPath(a.Path)
}

// ValidateAttachmentDescs validates a message's attachment list.
func ValidateAttachmentDescs(list []AttachmentDesc) error {
	if len(list) > MaxAttachmentsPerMessage {
		return fmt.Errorf("too many attachments: limit is %d", MaxAttachmentsPerMessage)
	}
	for _, a := range list {
		if err := ValidateAttachmentDesc(a); err != nil {
			return err
		}
	}
	return nil
}
