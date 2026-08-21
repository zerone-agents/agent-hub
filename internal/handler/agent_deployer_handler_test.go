package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/application/services"
	"control-panel/internal/domain/tenant"

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
}

func (f *fakeDeployer) Deploy(tenantID, name string, force bool, rotateKey bool) (*services.DeploymentDTO, error) {
	f.gotTenantID = tenantID
	f.gotName = name
	f.gotForce = force
	f.gotRotateKey = rotateKey
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

// Compile-time assertion that the production service satisfies the seam used
// by this test, so the test stays faithful to the real Deploy signature.
var _ deployerForTest = (*services.AgentDeployerService)(nil)
