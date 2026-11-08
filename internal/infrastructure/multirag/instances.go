package multirag

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"control-panel/internal/domain/provider"
)

var _ provider.MultiRAGInstances = (*Client)(nil)

// Include the credential scope so changing the service tenant cannot silently
// adopt another tenant's binding. Neither the token nor this digest is public.
func (c *Client) SyncTarget() string {
	sum := sha256.Sum256([]byte(c.baseURL + "\x00" + c.apiKey))
	return hex.EncodeToString(sum[:])
}

type instanceError struct{ status int }

func (e *instanceError) Error() string {
	return fmt.Sprintf("MultiRAG instance operation failed (HTTP %d); refresh to confirm the outcome", e.status)
}
func (e *instanceError) Rejected() bool {
	return e.status == 400 || e.status == 401 || e.status == 403 || e.status == 404 || e.status == 409 || e.status == 422
}

func (c *Client) instanceRequest(ctx context.Context, method, path string, input any, output any) error {
	var body io.Reader
	if input != nil {
		raw, err := json.Marshal(input)
		if err != nil {
			return fmt.Errorf("invalid MultiRAG instance request")
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+"/api/v1"+path, body)
	if err != nil {
		return fmt.Errorf("invalid MultiRAG instance URL")
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := *c.http
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("MultiRAG instance request outcome is unconfirmed; refresh before retrying")
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return fmt.Errorf("invalid MultiRAG instance response")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &instanceError{res.StatusCode}
	}
	var env struct {
		Code    *int            `json:"code"`
		RetCode *int            `json:"retcode"`
		Data    json.RawMessage `json:"data"`
	}
	if json.Unmarshal(raw, &env) != nil || (env.Code == nil && env.RetCode == nil) || (env.Code != nil && *env.Code != 0) || (env.RetCode != nil && *env.RetCode != 0) || len(env.Data) == 0 || bytes.Equal(env.Data, []byte("null")) {
		return fmt.Errorf("MultiRAG instance response did not confirm success")
	}
	if output != nil && json.Unmarshal(env.Data, output) != nil {
		return fmt.Errorf("invalid MultiRAG instance response")
	}
	return nil
}

func instancePath(factory, instance string) string {
	path := "/providers/" + url.PathEscape(factory) + "/instances"
	if instance != "" {
		path += "/" + url.PathEscape(instance)
	}
	return path
}

func inventory[T any](ctx context.Context, c *Client, path string, identity func(T) string) ([]T, error) {
	items := []T{}
	seen := map[string]bool{}
	total := -1
	for page := 1; page <= 101; page++ {
		var result struct {
			Items *[]T `json:"items"`
			Total *int `json:"total"`
			Page  int  `json:"page"`
			Size  int  `json:"page_size"`
		}
		separator := "?"
		if bytes.Contains([]byte(path), []byte("?")) {
			separator = "&"
		}
		if err := c.instanceRequest(ctx, http.MethodGet, path+separator+"page="+strconv.Itoa(page)+"&page_size=100", nil, &result); err != nil {
			return nil, err
		}
		if result.Items == nil || result.Total == nil || *result.Total < 0 || *result.Total > 10000 || result.Page != page || result.Size != 100 || len(*result.Items) > 100 {
			return nil, fmt.Errorf("incomplete MultiRAG model inventory")
		}
		if total < 0 {
			total = *result.Total
		}
		if total != *result.Total {
			return nil, fmt.Errorf("MultiRAG model inventory changed during read")
		}
		for _, item := range *result.Items {
			id := identity(item)
			if id == "" || seen[id] {
				return nil, fmt.Errorf("invalid MultiRAG model identity")
			}
			seen[id] = true
			items = append(items, item)
		}
		if len(items) > total {
			return nil, fmt.Errorf("invalid MultiRAG inventory total")
		}
		if len(items) == total {
			return items, nil
		}
		if len(*result.Items) != 100 {
			return nil, fmt.Errorf("incomplete MultiRAG model inventory")
		}
	}
	return nil, fmt.Errorf("MultiRAG inventory exceeds scan limit")
}

func (c *Client) ListInstances(ctx context.Context, factory string) ([]provider.RemoteInstance, error) {
	// Probe a collection route; a missing provider resource is not proof of an
	// old API. No writes have happened when legacy compatibility is selected.
	type factoryRow struct {
		Name string `json:"name"`
	}
	factories, err := inventory(ctx, c, "/providers?available=true", func(row factoryRow) string { return row.Name })
	if failure, ok := err.(*instanceError); ok && (failure.status == 404 || failure.status == 405) {
		return nil, provider.ErrInstanceAPIUnavailable
	}
	if err != nil {
		return nil, err
	}
	found := false
	for _, row := range factories {
		if row.Name == factory {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("MultiRAG provider does not support instance registration")
	}
	instances, err := inventory(ctx, c, instancePath(factory, ""), func(row provider.RemoteInstance) string {
		if row.ProviderName != factory || row.Name == "" || (row.Status != "active" && row.Status != "inactive") {
			return ""
		}
		return row.ID
	})
	if failure, ok := err.(*instanceError); ok && failure.status == 404 {
		return []provider.RemoteInstance{}, nil
	}
	return instances, err
}

func (c *Client) VerifyInstance(ctx context.Context, factory string, input provider.InstanceCandidate) error {
	var result struct {
		Verified bool   `json:"verified"`
		Model    string `json:"model_name"`
		Type     string `json:"model_type"`
	}
	if err := c.instanceRequest(ctx, http.MethodPost, "/providers/"+url.PathEscape(factory)+"/connection", input, &result); err != nil {
		return err
	}
	if !result.Verified {
		return fmt.Errorf("MultiRAG provider verification was not confirmed")
	}
	for _, model := range input.CustomModels {
		if model.ModelName == result.Model {
			for _, capability := range model.ModelTypes {
				if result.Type == capability {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("MultiRAG verified a different candidate")
}

func (c *Client) CreateInstance(ctx context.Context, factory string, input provider.InstanceInput) (*provider.RemoteInstance, error) {
	var row provider.RemoteInstance
	if err := c.instanceRequest(ctx, http.MethodPost, instancePath(factory, ""), input, &row); err != nil {
		return nil, err
	}
	if !validInstance(row, factory, input.InstanceName, input.InstanceCandidate) {
		return nil, fmt.Errorf("MultiRAG instance creation is unconfirmed")
	}
	return &row, nil
}

func validInstance(row provider.RemoteInstance, factory, name string, input provider.InstanceCandidate) bool {
	return row.ID != "" && row.ProviderName == factory && row.Name == name && row.Status == "active" && row.BaseURL == input.BaseURL && row.Region == input.Region && row.HasAPIKey == (input.APIKey != "")
}

func (c *Client) UpdateInstance(ctx context.Context, factory, name string, input provider.InstanceCandidate) (*provider.RemoteInstance, error) {
	body := map[string]any{"api_key": input.APIKey, "base_url": input.BaseURL, "region": input.Region}
	if input.APIKey == "" {
		delete(body, "api_key")
		body["clear_api_key"] = true
	}
	var row provider.RemoteInstance
	if err := c.instanceRequest(ctx, http.MethodPatch, instancePath(factory, name), body, &row); err != nil {
		return nil, err
	}
	if !validInstance(row, factory, name, input) {
		return nil, fmt.Errorf("MultiRAG instance update is unconfirmed")
	}
	return &row, nil
}

func (c *Client) ListInstanceModels(ctx context.Context, factory, name string) ([]provider.RemoteModel, error) {
	return inventory(ctx, c, instancePath(factory, name)+"/models", func(row provider.RemoteModel) string {
		if row.Name == "" || row.ProviderName != factory || row.InstanceName != name || row.InstanceID == "" || !provider.ValidModelReference(row.Reference) || row.ID.String() != row.Reference || len(row.ModelTypes) == 0 || (row.EffectiveStatus != "active" && row.EffectiveStatus != "inactive") {
			return ""
		}
		return row.Reference
	})
}

func (c *Client) AddInstanceModel(ctx context.Context, factory, name string, input provider.ModelRegistration) (*provider.RemoteModel, error) {
	var row provider.RemoteModel
	if err := c.instanceRequest(ctx, http.MethodPost, instancePath(factory, name)+"/models", input, &row); err != nil {
		return nil, err
	}
	if row.Name != input.ModelName || row.ProviderName != factory || row.InstanceName != name || !provider.ValidModelReference(row.Reference) || row.ID.String() != row.Reference || row.EffectiveStatus != "active" {
		return nil, fmt.Errorf("MultiRAG model registration is unconfirmed")
	}
	return &row, nil
}
