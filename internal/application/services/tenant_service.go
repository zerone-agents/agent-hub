package services

import (
	"fmt"

	"control-panel/internal/auth"
	"control-panel/internal/domain/tenant"
	repository "control-panel/internal/infrastructure/persistence"
)

type TenantService struct {
	repo *repository.TenantRepository
}

func NewTenantService() *TenantService {
	return &TenantService{
		repo: repository.NewTenantRepository(),
	}
}

type CreateTenantRequest struct {
	Name    string
	Domain  string
	Plan    string
	OwnerID string
}

type CreateTenantResponse struct {
	Tenant *tenant.Tenant
	OrgID  string
}

func (s *TenantService) CreateTenant(req *CreateTenantRequest) (*CreateTenantResponse, error) {
	tenant := &tenant.Tenant{
		Name:   req.Name,
		Domain: req.Domain,
		Status: "active",
		Plan:   req.Plan,
	}

	if err := s.repo.Create(tenant); err != nil {
		return nil, fmt.Errorf("failed to create tenant: %w", err)
	}

	orgName := fmt.Sprintf("tenant-%d", tenant.ID)
	if err := auth.CreateOrganization(orgName, req.Name, req.OwnerID); err != nil {
		return nil, fmt.Errorf("failed to create Casdoor organization: %w", err)
	}

	return &CreateTenantResponse{
		Tenant: tenant,
		OrgID:  orgName,
	}, nil
}

func (s *TenantService) GetTenant(id int64) (*tenant.Tenant, error) {
	return s.repo.GetByID(id)
}

func (s *TenantService) ListTenants() ([]*tenant.Tenant, error) {
	return s.repo.List()
}

func (s *TenantService) UpdateTenant(tenant *tenant.Tenant) error {
	return s.repo.Update(tenant)
}

func (s *TenantService) DeleteTenant(id int64) error {
	orgName := fmt.Sprintf("tenant-%d", id)
	if err := auth.DeleteOrganization(orgName); err != nil {
		return fmt.Errorf("failed to delete Casdoor organization: %w", err)
	}

	return s.repo.Delete(id)
}
