package azure

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type AzureClient struct {
	subscriptionId string
	resourceGroup  string
	vmName         string
}

// vmResponse represents the Azure VM API response structure
type vmResponse struct {
	Properties vmProperties `json:"properties"`
}

type vmProperties struct {
	AdditionalCapabilities *additionalCapabilities `json:"additionalCapabilities,omitempty"`
}

type additionalCapabilities struct {
	HibernationEnabled *bool `json:"hibernationEnabled,omitempty"`
}

func NewAzureClient(subscriptionId, resourceGroup, vmName string) *AzureClient {
	return &AzureClient{
		subscriptionId: subscriptionId,
		resourceGroup:  resourceGroup,
		vmName:         vmName,
	}
}

// HibernateVM sends a hibernation request to Azure for the VM
func (c *AzureClient) HibernateVM(ctx context.Context) error {
	// Get the access token
	token, err := GetManagedIdentityToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to get managed identity token: %w", err)
	}

	// Build the hibernation API URL
	// https://management.azure.com/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/virtualMachines/{vmName}/deallocate?api-version=2024-07-01&hibernate=true
	url := fmt.Sprintf(
		"%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s/deallocate?api-version=%s&hibernate=true",
		azureManagementEndpoint,
		c.subscriptionId,
		c.resourceGroup,
		c.vmName,
		computeApiVersion,
	)

	// Create the POST request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader([]byte{}))
	if err != nil {
		return fmt.Errorf("failed to create hibernation request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send hibernation request to %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Read response body for error details
	body, _ := io.ReadAll(resp.Body)

	// Check response status
	// 200 OK or 202 Accepted are both valid responses
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("hibernation request failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CheckHibernationEnabled checks if hibernation is enabled on the VM via Azure API
func (c *AzureClient) CheckHibernationEnabled(ctx context.Context) (bool, error) {
	// Get the access token
	token, err := GetManagedIdentityToken(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get managed identity token: %w", err)
	}

	// Build the VM properties API URL
	url := fmt.Sprintf(
		"%s/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/virtualMachines/%s?api-version=%s",
		azureManagementEndpoint,
		c.subscriptionId,
		c.resourceGroup,
		c.vmName,
		computeApiVersion,
	)

	// Create the GET request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false, fmt.Errorf("failed to create VM properties request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to get VM properties from %s: %w", url, err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to read VM properties response: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("VM properties request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the JSON response properly
	var vmResp vmResponse
	if err := json.Unmarshal(body, &vmResp); err != nil {
		return false, fmt.Errorf("failed to parse VM properties JSON: %w", err)
	}

	// Check if hibernation is enabled
	// The field is nested: properties.additionalCapabilities.hibernationEnabled
	if vmResp.Properties.AdditionalCapabilities == nil {
		// additionalCapabilities not present means hibernation is not configured
		return false, nil
	}

	if vmResp.Properties.AdditionalCapabilities.HibernationEnabled == nil {
		// hibernationEnabled not present means hibernation is not enabled
		return false, nil
	}

	// Return the actual boolean value
	return *vmResp.Properties.AdditionalCapabilities.HibernationEnabled, nil
}
