package tenant

import (
	"time"
)

type Tenant struct {
	ID        int64     `json:"id" gorm:"primaryKey;type:bigint;autoIncrement"`
	Name      string    `json:"name" gorm:"type:varchar(255);not null"`
	Domain    string    `json:"domain" gorm:"type:varchar(255);uniqueIndex"`
	Status    string    `json:"status" gorm:"type:varchar(50);default:'active'"`
	Plan      string    `json:"plan" gorm:"type:varchar(50);default:'free'"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type User struct {
	ID            int64     `json:"id" gorm:"primaryKey;type:bigint;autoIncrement"`
	TenantID      int64     `json:"tenant_id" gorm:"type:bigint;not null;index"`
	CasdoorUserID string    `json:"casdoor_user_id" gorm:"type:varchar(255);not null"`
	Name          string    `json:"name" gorm:"type:varchar(255)"`
	Email         string    `json:"email" gorm:"type:varchar(255)"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type ServiceDeployment struct {
	ID          int64     `json:"id" gorm:"primaryKey;type:bigint;autoIncrement"`
	TenantID    int64     `json:"tenant_id" gorm:"type:bigint;not null;index"`
	ServiceName string    `json:"service_name" gorm:"type:varchar(255);not null"`
	ServiceType string    `json:"service_type" gorm:"type:varchar(50);default:'shared'"`
	EndpointURL string    `json:"endpoint_url" gorm:"type:varchar(512)"`
	IsActive    bool      `json:"is_active" gorm:"default:true"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Resource struct {
	ID          int64     `json:"id" gorm:"primaryKey;type:bigint;autoIncrement"`
	TenantID    int64     `json:"tenant_id" gorm:"type:bigint;not null;index"`
	Name        string    `json:"name" gorm:"type:varchar(255);not null"`
	Description string    `json:"description" gorm:"type:text"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
