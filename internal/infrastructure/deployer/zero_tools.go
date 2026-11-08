package deployer

import (
	"context"
	"net/http"
	"time"
)

// An old deployer ignores unknown JSON fields. Require an explicit capability
// before sending zeroTools, rather than silently deploying a tools-enabled agent.
func (c *Client) SupportsZeroTools(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	resp, err := c.doRequest(ctx, http.MethodGet, "/api/v1/capabilities", nil)
	if err != nil {
		return false, err
	}
	var caps struct {
		ExplicitZeroTools bool `json:"explicitZeroTools"`
		ImageInspection   bool `json:"zeroToolsImageInspection"`
	}
	if err := decodeSuccess(resp, &caps); err != nil {
		return false, nil
	}
	return caps.ExplicitZeroTools && caps.ImageInspection, nil
}
