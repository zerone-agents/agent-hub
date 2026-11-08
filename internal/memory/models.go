package memory

import (
	"gorm.io/gorm"
	"time"
)

type lifecycleRow struct {
	ID            string `gorm:"primaryKey;size:64"`
	UserKey       string `gorm:"index;size:64"`
	AppKey        string `gorm:"index;size:64"`
	State         string `gorm:"size:24"`
	Revision      uint64
	Fence         uint64
	WrappedKey    []byte `gorm:"type:mediumblob"`
	ScopePayload  []byte `gorm:"type:mediumblob"`
	DocumentID    string `gorm:"size:128"`
	DocumentState string `gorm:"size:24"`
	Profile       []byte `gorm:"type:mediumblob"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (lifecycleRow) TableName() string { return "memory_lifecycles" }

type recordRow struct {
	ID          string `gorm:"primaryKey;size:64"`
	ScopeKey    string `gorm:"index;size:64"`
	Kind        string `gorm:"index;size:16"`
	Version     uint64
	Status      string `gorm:"index;size:24"`
	SyncStatus  string `gorm:"size:24"`
	Fingerprint string `gorm:"index;size:64"`
	Payload     []byte `gorm:"type:mediumblob"`
	ChunkID     string `gorm:"index;size:128"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (recordRow) TableName() string { return "memory_records" }

type operationRow struct {
	ID          string `gorm:"primaryKey;size:64"`
	ScopeKey    string `gorm:"index;size:64"`
	RequestHash string `gorm:"size:64"`
	Action      string `gorm:"size:24"`
	Result      []byte `gorm:"type:mediumblob"`
	CreatedAt   time.Time
}

func (operationRow) TableName() string { return "memory_operations" }

type taskRow struct {
	ID        string `gorm:"primaryKey;size:64"`
	ScopeKey  string `gorm:"index;size:64"`
	Fence     uint64
	Status    string `gorm:"index;size:24"`
	Payload   []byte `gorm:"type:mediumblob"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (taskRow) TableName() string { return "memory_tasks" }

type outboxRow struct {
	ID         string `gorm:"primaryKey;size:64"`
	ScopeKey   string `gorm:"index;size:64"`
	AppKey     string `gorm:"index;size:64"`
	RecordID   string `gorm:"size:64"`
	Version    uint64
	Fence      uint64
	Action     string `gorm:"size:24"`
	ChunkID    string `gorm:"size:128"`
	Payload    []byte `gorm:"type:mediumblob"`
	Status     string `gorm:"index;size:24"`
	Attempts   int
	LeaseUntil *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (outboxRow) TableName() string { return "memory_outbox" }

type userRow struct {
	ID        string `gorm:"primaryKey;size:64"`
	CurrentID string `gorm:"size:64"`
	State     string `gorm:"size:24"`
}

func (userRow) TableName() string { return "memory_users" }

type conversationRow struct {
	ID           string `gorm:"primaryKey;size:64"`
	ScopeKey     string `gorm:"uniqueIndex:memory_conversation_operation;size:64"`
	OperationKey string `gorm:"uniqueIndex:memory_conversation_operation;size:64"`
	RequestHash  string `gorm:"size:64"`
	Payload      []byte `gorm:"type:mediumblob"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (conversationRow) TableName() string { return "memory_conversations" }

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&userRow{}, &lifecycleRow{}, &recordRow{}, &operationRow{}, &taskRow{}, &outboxRow{}, &conversationRow{})
}
