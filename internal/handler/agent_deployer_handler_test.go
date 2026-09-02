package handler

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"
	"control-panel/internal/infrastructure/deployer"

	"github.com/gin-gonic/gin"
)

// deployerForTest is the subset of *services.AgentDeployerService that the
// deploy handler depends on. The production *services.AgentDeployerService
// implicitly satisfies it (see Deploy signature). It exists only so this
// test can inject a fake that records forwarded arguments — the production
// AgentHandler holds a concrete *services.AgentDeployerService, which cannot
// be mocked directly without a DB, so we exercise the identical query-parsing
// path here.
type deployerForTest interface {
	Deploy(tenantID, name string, force bool, rotateKey bool) (*services.DeploymentDTO, error)
}

// fakeDeployer records the arguments passed to Deploy.
type fakeDeployer struct {
	gotTenantID  string
	gotName      string
	gotForce     bool
	gotRotateKey bool
	// err, when non-nil, is returned from Deploy verbatim (already wrapped
	// by the caller if the scenario needs a service-layer chain).
	err error
}

func (f *fakeDeployer) Deploy(tenantID, name string, force bool, rotateKey bool) (*services.DeploymentDTO, error) {
	f.gotTenantID = tenantID
	f.gotName = name
	f.gotForce = force
	f.gotRotateKey = rotateKey
	if f.err != nil {
		return nil, f.err
	}
	return &services.DeploymentDTO{Status: "running"}, nil
}

// deployHandlerForTest mirrors AgentHandler.DeployAgent's query-parsing logic
// verbatim, delegating to the injectable deployerForTest seam.
func deployHandlerForTest(dep deployerForTest) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		force := c.Query("force") == "true"
		rotateKey := c.Query("rotate_key") == "true"

		resp, err := dep.Deploy(tenant.GetTenantID(c), name, force, rotateKey)
		if err != nil {
			respondError(c, deployerErrorStatus(err), err.Error())
			return
		}
		respondSuccess(c, resp)
	}
}

func setupDeployRouter(dep deployerForTest) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/admin/agents/:name/deploy", deployHandlerForTest(dep))
	return r
}

func TestDeployAgent_RotateKeyQueryParsing(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		wantRotateKey bool
	}{
		{name: "rotate_key=true", query: "?rotate_key=true", wantRotateKey: true},
		{name: "rotate_key=false", query: "?rotate_key=false", wantRotateKey: false},
		{name: "rotate_key absent", query: "", wantRotateKey: false},
		{name: "rotate_key non-true value", query: "?rotate_key=1", wantRotateKey: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeDeployer{}
			router := setupDeployRouter(fake)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/agents/demo/deploy"+tc.query, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
			}
			if fake.gotName != "demo" {
				t.Errorf("Deploy name = %q, want %q", fake.gotName, "demo")
			}
			if fake.gotRotateKey != tc.wantRotateKey {
				t.Errorf("Deploy rotateKey = %v, want %v", fake.gotRotateKey, tc.wantRotateKey)
			}
		})
	}
}

// TestDeployAgent_DeployerHTTPErrorMapping pins the deployer v3 error
// contract: typed deployer.HTTPError statuses pass through as-is (4xx
// protocol rejections, 503 runtime version floor) while upstream 5xx
// failures surface as 502; non-HTTPError errors keep the legacy string
// matching. HTTPErrors are wrapped with %w exactly like
// AgentDeployerService.Deploy does ("deploy agent failed: %w"), so these
// cases also prove errors.As penetration through the service wrapping.
func TestDeployAgent_DeployerHTTPErrorMapping(t *testing.T) {
	wrap := func(httpErr *deployer.HTTPError) error {
		return fmt.Errorf("deploy agent failed: %w", httpErr)
	}

	tests := []struct {
		name       string
		deployErr  error
		wantStatus int
		wantBody   string
	}{
		{
			name: "deployer 400 protocol rejection passes through",
			deployErr: wrap(&deployer.HTTPError{
				StatusCode: http.StatusBadRequest,
				Message:    `agent "researcher": model is a runtime-global field and may only be set on the root agent`,
			}),
			wantStatus: http.StatusBadRequest,
			wantBody:   "model is a runtime-global field",
		},
		{
			name: "deployer 503 runtime version floor passes through",
			deployErr: wrap(&deployer.HTTPError{
				StatusCode: http.StatusServiceUnavailable,
				Message:    "runtime image v2.5.9 is below the required floor v2.6.0",
			}),
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   "below the required floor v2.6.0",
		},
		{
			name: "deployer 500 maps to 502",
			deployErr: wrap(&deployer.HTTPError{
				StatusCode: http.StatusInternalServerError,
				Message:    "docker daemon unavailable",
			}),
			wantStatus: http.StatusBadGateway,
			wantBody:   "docker daemon unavailable",
		},
		{
			name:       "legacy string path: agent not found still 400",
			deployErr:  errors.New("agent not found"),
			wantStatus: http.StatusBadRequest,
			wantBody:   "agent not found",
		},
		{
			name:       "legacy fallback: unknown error still 500",
			deployErr:  errors.New("some unexpected failure"),
			wantStatus: http.StatusInternalServerError,
			wantBody:   "some unexpected failure",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeDeployer{err: tc.deployErr}
			router := setupDeployRouter(fake)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/agents/demo/deploy", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("response body %q does not contain deployer message %q", rec.Body.String(), tc.wantBody)
			}
		})
	}
}

// Compile-time assertion that the production service satisfies the seam used
// by this test, so the test stays faithful to the real Deploy signature.
var _ deployerForTest = (*services.AgentDeployerService)(nil)
