// SPDX-License-Identifier: Apache-2.0

package jira

import (
	"context"
	"fmt"
	"net/http"
)

// Mode returns the client's resolved platform mode (Cloud or Data Center). It lets
// callers outside the package name the credential style and render the mode without
// re-inspecting URLs or environment variables.
func (c *Client) Mode() Mode {
	return c.mode
}

// CheckAuth performs a read-only identity/reachability probe against the Jira
// instance by GETting /rest/api/<v>/myself. It sends no body and creates nothing.
// A successful (2xx) response returns nil; a non-2xx response surfaces as an
// *APIError (via doRequest), so callers can branch the diagnosis on StatusCode.
func (c *Client) CheckAuth(ctx context.Context) error {
	endpoint := fmt.Sprintf("/rest/api/%s/myself", c.apiVersion)
	if err := c.doRequest(ctx, http.MethodGet, endpoint, nil, nil); err != nil {
		return fmt.Errorf("check auth: %w", err)
	}
	return nil
}

// CheckProjectAccess performs a read-only access probe for the given project key by
// GETting /rest/api/<v>/project/<projectKey>. It validates the key format before
// the network call, sends no body, and creates nothing. A successful (2xx) response
// returns nil; a non-2xx response surfaces as an *APIError (via doRequest).
func (c *Client) CheckProjectAccess(ctx context.Context, projectKey string) error {
	if !projectKeyRE.MatchString(projectKey) {
		return fmt.Errorf("invalid project key %q: must match ^[A-Z][A-Z0-9]+$", projectKey)
	}
	endpoint := fmt.Sprintf("/rest/api/%s/project/%s", c.apiVersion, projectKey)
	if err := c.doRequest(ctx, http.MethodGet, endpoint, nil, nil); err != nil {
		return fmt.Errorf("check project access: %w", err)
	}
	return nil
}
