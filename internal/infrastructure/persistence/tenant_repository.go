package repository

import (
	"control-panel/internal/domain/tenant"
	"control-panel/pkg/database"

	"gorm.io/gorm"
)

// TenantRepository provides database access for tenant entities.
type TenantRepository struct {
	db *gorm.DB
}

// NewTenantRepository creates a new TenantRepository using the default database connection.
func NewTenantRepository() *TenantRepository {
	return &TenantRepository{
		db: database.GetDB(),
	}
}

// Create inserts a new tenant record.
func (r *TenantRepository) Create(tenant *tenant.Tenant) error {
	return r.db.Create(tenant).Error
}

// GetByID retrieves a tenant by its primary key.
func (r *TenantRepository) GetByID(id int64) (*tenant.Tenant, error) {
	return r.findFirst("id = ?", id)
}

// GetByDomain retrieves a tenant by its domain name.
func (r *TenantRepository) GetByDomain(domain string) (*tenant.Tenant, error) {
	return r.findFirst("domain = ?", domain)
}

// findFirst is a helper that retrieves the first tenant matching the given query.
func (r *TenantRepository) findFirst(query string, args ...interface{}) (*tenant.Tenant, error) {
	var t tenant.Tenant
	conds := append([]interface{}{query}, args...)
	err := r.db.First(&t, conds...).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// List returns all tenants.
func (r *TenantRepository) List() ([]*tenant.Tenant, error) {
	var tenants []*tenant.Tenant
	err := r.db.Find(&tenants).Error
	return tenants, err
}

// Update saves changes to an existing tenant record.
func (r *TenantRepository) Update(tenant *tenant.Tenant) error {
	return r.db.Save(tenant).Error
}

// Delete removes a tenant by its primary key.
func (r *TenantRepository) Delete(id int64) error {
	return r.db.Delete(&tenant.Tenant{}, "id = ?", id).Error
}

// GetServiceDeployments returns all service deployments for a given tenant.
func (r *TenantRepository) GetServiceDeployments(tenantID int64) ([]*tenant.ServiceDeployment, error) {
	var deployments []*tenant.ServiceDeployment
	err := r.db.Where("tenant_id = ?", tenantID).Find(&deployments).Error
	return deployments, err
}
