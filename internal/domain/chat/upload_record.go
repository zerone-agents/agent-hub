package chat

import "time"

// UploadRecord binds a runtime attachment descriptor to the hub session that
// uploaded it. ID is the runtime-generated descriptor id (unguessable UUID) —
// the authorization anchor; message content parts are display-only.
type UploadRecord struct {
	ID        string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	TenantID  string `json:"tenant_id" gorm:"type:varchar(64);index;not null;default:''"`
	SessionID string `json:"session_id" gorm:"type:varchar(255);index;not null"`
	UserID    string `json:"user_id" gorm:"type:varchar(255);not null"`
	Name      string `json:"name" gorm:"type:varchar(255)"`
	Mime      string `json:"mime" gorm:"type:varchar(127)"`
	Size      int64  `json:"size"`
	Path      string `json:"path" gorm:"type:varchar(512)"`
	// ContainerID is the deployer-reported runtime container id captured when
	// the upload request was issued — the immutable deployment-generation
	// anchor (issue #94 review R3). Docker recreates the id on every
	// recreate/redeploy but keeps it across in-place restarts, mirroring the
	// on-disk lifetime of `.zerone-uploads` files exactly. Records are only
	// honored while the deployer reports the SAME container id (exact match,
	// no time tolerance).
	ContainerID string    `json:"container_id" gorm:"type:varchar(128);index"`
	CreatedAt   time.Time `json:"created_at"`
}

func (UploadRecord) TableName() string { return "chat_upload_records" }
