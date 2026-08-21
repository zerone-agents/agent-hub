package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"control-panel/internal/domain/agent"
	"control-panel/internal/domain/provider"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testEncryptionKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type mockAgentRuntimeAuthRepo struct {
	listAllFunc func() ([]*agent.AgentConfig, error)
}

func (m *mockAgentRuntimeAuthRepo) ListAllUnscoped() ([]*agent.AgentConfig, error) {
	if m.listAllFunc != nil {
		return m.listAllFunc()
	}
	return nil, errors.New("not implemented")
}

func TestAgentRuntimeAuthMiddleware_ValidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rawToken := "agent-runtime-token"
	encryptedToken, err := provider.Encrypt(rawToken, testEncryptionKey)
	require.NoError(t, err)

	expectedAgent := &agent.AgentConfig{
		ID:           1,
		Name:         "test-agent",
		RuntimeToken: encryptedToken,
	}

	repo := &mockAgentRuntimeAuthRepo{
		listAllFunc: func() ([]*agent.AgentConfig, error) {
			return []*agent.AgentConfig{expectedAgent}, nil
		},
	}

	mw := agentRuntimeAuthMiddleware(testEncryptionKey, repo)

	router := gin.New()
	var capturedAgent *agent.AgentConfig
	router.GET("/protected", mw, func(c *gin.Context) {
		a, ok := AgentFromContext(c)
		require.True(t, ok)
		capturedAgent = a
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+rawToken)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	require.NotNil(t, capturedAgent)
	assert.Equal(t, expectedAgent.ID, capturedAgent.ID)
	assert.Equal(t, expectedAgent.Name, capturedAgent.Name)
}

func TestAgentRuntimeAuthMiddleware_InvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rawToken := "agent-runtime-token"
	encryptedToken, err := provider.Encrypt(rawToken, testEncryptionKey)
	require.NoError(t, err)

	repo := &mockAgentRuntimeAuthRepo{
		listAllFunc: func() ([]*agent.AgentConfig, error) {
			return []*agent.AgentConfig{
				{ID: 1, Name: "test-agent", RuntimeToken: encryptedToken},
			}, nil
		},
	}

	mw := agentRuntimeAuthMiddleware(testEncryptionKey, repo)

	router := gin.New()
	router.GET("/protected", mw, func(c *gin.Context) {
		t.Error("handler should not be called for invalid token")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAgentRuntimeAuthMiddleware_MissingHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &mockAgentRuntimeAuthRepo{
		listAllFunc: func() ([]*agent.AgentConfig, error) {
			return []*agent.AgentConfig{}, nil
		},
	}

	mw := agentRuntimeAuthMiddleware(testEncryptionKey, repo)

	router := gin.New()
	router.GET("/protected", mw, func(c *gin.Context) {
		t.Error("handler should not be called when header is missing")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAgentRuntimeAuthMiddleware_MalformedHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	repo := &mockAgentRuntimeAuthRepo{
		listAllFunc: func() ([]*agent.AgentConfig, error) {
			return []*agent.AgentConfig{}, nil
		},
	}

	mw := agentRuntimeAuthMiddleware(testEncryptionKey, repo)

	cases := []string{
		"Basic dXNlcjpwYXNzd29yZA==",
		"Bearer",
		"Bearer   ",
		"Bearertoken",
	}

	for _, header := range cases {
		router := gin.New()
		router.GET("/protected", mw, func(c *gin.Context) {
			t.Errorf("handler should not be called for malformed header %q", header)
			c.Status(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set("Authorization", header)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code, "header: %q", header)
	}
}
