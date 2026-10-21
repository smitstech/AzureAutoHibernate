package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	// Azure Instance Metadata Service endpoints
	imdsTokenEndpoint    = "http://169.254.169.254/metadata/identity/oauth2/token"
	imdsInstanceEndpoint = "http://169.254.169.254/metadata/instance"

	// Azure API endpoints
	azureManagementEndpoint = "https://management.azure.com"
	azureLoginEndpoint      = "https://login.microsoftonline.com"

	// IMDS API versions
	imdsTokenApiVersion    = "2018-02-01"
	imdsInstanceApiVersion = "2021-02-01"

	// Azure Resource Manager API versions
	computeApiVersion = "2024-07-01"

	// Legacy aliases for backward compatibility
	apiVersion         = imdsTokenApiVersion
	instanceApiVersion = imdsInstanceApiVersion
	resource           = azureManagementEndpoint + "/"
)

type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    string `json:"expires_in"`
	ExpiresOn    string `json:"expires_on"`
	NotBefore    string `json:"not_before"`
	Resource     string `json:"resource"`
	TokenType    string `json:"token_type"`
}

// IMDSComputeResponse represents the compute metadata response from Azure IMDS
type IMDSComputeResponse struct {
	SubscriptionID    string `json:"subscriptionId"`
	ResourceGroupName string `json:"resourceGroupName"`
	Name              string `json:"name"`
}

// GetManagedIdentityToken retrieves an access token using the VM's System Managed Identity
func GetManagedIdentityToken(ctx context.Context) (string, error) {
	// Build the request URL
	params := url.Values{}
	params.Add("api-version", apiVersion)
	params.Add("resource", resource)

	reqUrl := fmt.Sprintf("%s?%s", imdsTokenEndpoint, params.Encode())

	// Create the request
	req, err := http.NewRequestWithContext(ctx, "GET", reqUrl, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set the required Metadata header
	req.Header.Set("Metadata", "true")

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get token from IMDS: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("IMDS returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the JSON response
	var tokenResp TokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("access token is empty in response")
	}

	return tokenResp.AccessToken, nil
}

// spTokenResponse is the OAuth2 client-credentials token reply (v2 endpoint).
// Note: expires_in is a JSON number here (a string in the IMDS v1 reply).
type spTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// GetServicePrincipalToken retrieves an access token using OAuth2 client
// credentials against the public login endpoint — no IMDS required, so it
// works even when link-local/Azure-fabric traffic is intercepted (VPN,
// exit node, filter drivers).
func GetServicePrincipalToken(ctx context.Context, tenantID, clientID, clientSecret string) (string, error) {
	tokenURL := fmt.Sprintf("%s/%s/oauth2/v2.0/token", azureLoginEndpoint, tenantID)

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("scope", azureManagementEndpoint+"/.default")

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get token from login endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("login endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tr spTokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("failed to parse token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("access_token is empty in login response")
	}
	return tr.AccessToken, nil
}

// VMMetadata contains the VM information retrieved from IMDS
type VMMetadata struct {
	SubscriptionId string
	ResourceGroup  string
	VMName         string
}

// GetVMMetadata retrieves VM metadata from Azure IMDS
func GetVMMetadata(ctx context.Context) (*VMMetadata, error) {
	// Build the request URL
	params := url.Values{}
	params.Add("api-version", instanceApiVersion)
	params.Add("format", "json")

	reqUrl := fmt.Sprintf("%s/compute?%s", imdsInstanceEndpoint, params.Encode())

	// Create the request
	req, err := http.NewRequestWithContext(ctx, "GET", reqUrl, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set the required Metadata header
	req.Header.Set("Metadata", "true")

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata from IMDS (endpoint: %s): %w", imdsInstanceEndpoint, err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("IMDS returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse the JSON response
	var computeResp IMDSComputeResponse
	if err := json.Unmarshal(body, &computeResp); err != nil {
		return nil, fmt.Errorf("failed to parse compute metadata response: %w", err)
	}

	// Validate required fields
	if computeResp.SubscriptionID == "" {
		return nil, fmt.Errorf("subscriptionId is empty in response")
	}
	if computeResp.ResourceGroupName == "" {
		return nil, fmt.Errorf("resourceGroupName is empty in response")
	}
	if computeResp.Name == "" {
		return nil, fmt.Errorf("VM name is empty in response")
	}

	return &VMMetadata{
		SubscriptionId: computeResp.SubscriptionID,
		ResourceGroup:  computeResp.ResourceGroupName,
		VMName:         computeResp.Name,
	}, nil
}

// HibernationCapabilityResult contains the results of hibernation capability testing
type HibernationCapabilityResult struct {
	IMDSAvailable       bool
	IMDSError           error
	VMMetadata          *VMMetadata
	TokenSuccess        bool
	TokenError          error
	HibernationEnabled  bool
	HibernationAPIError error
}

// TestHibernationCapability checks if the VM can be hibernated via Azure
// This tests IMDS connectivity, Managed Identity configuration, and VM hibernation capability
func TestHibernationCapability(ctx context.Context) *HibernationCapabilityResult {
	result := &HibernationCapabilityResult{}

	// Test 1: IMDS connectivity and VM metadata retrieval
	vmMetadata, err := GetVMMetadata(ctx)
	if err != nil {
		result.IMDSAvailable = false
		result.IMDSError = err
		return result
	}

	result.IMDSAvailable = true
	result.VMMetadata = vmMetadata

	// Test 2: Managed Identity token retrieval
	_, err = GetManagedIdentityToken(ctx)
	if err != nil {
		result.TokenSuccess = false
		result.TokenError = err
		return result
	}

	result.TokenSuccess = true

	// Test 3: Check if hibernation is actually enabled on the VM via Azure API
	client := NewAzureClient(vmMetadata.SubscriptionId, vmMetadata.ResourceGroup, vmMetadata.VMName)
	hibernationEnabled, err := client.CheckHibernationEnabled(ctx)
	if err != nil {
		result.HibernationEnabled = false
		result.HibernationAPIError = err
		return result
	}

	result.HibernationEnabled = hibernationEnabled
	return result
}
