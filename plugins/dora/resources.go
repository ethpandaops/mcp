package dora

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/mcp/pkg/plugin"
	"github.com/ethpandaops/mcp/pkg/resource"
	"github.com/ethpandaops/mcp/pkg/types"
)

// NetworkInfo represents a network with Dora explorer.
type NetworkInfo struct {
	Name    string `json:"name"`
	DoraURL string `json:"dora_url"`
	Status  string `json:"status"`
}

// NetworksListResponse is the response for dora://networks.
type NetworksListResponse struct {
	Description string        `json:"description"`
	Networks    []NetworkInfo `json:"networks"`
	Usage       string        `json:"usage"`
}

// NetworkOverviewResponse is the response for dora://network/{name}.
type NetworkOverviewResponse struct {
	Network  string        `json:"network"`
	DoraURL  string        `json:"dora_url"`
	Overview *DoraOverview `json:"overview,omitempty"`
	Error    string        `json:"error,omitempty"`
	Links    *NetworkLinks `json:"links"`
}

// NetworkLinks contains useful deep links for a network.
type NetworkLinks struct {
	Validators string `json:"validators"`
	Epochs     string `json:"epochs"`
	Slots      string `json:"slots"`
}

// DoraOverview contains network overview data from Dora API.
type DoraOverview struct {
	CurrentEpoch         int64   `json:"current_epoch"`
	CurrentSlot          int64   `json:"current_slot"`
	FinalizedEpoch       int64   `json:"finalized_epoch,omitempty"`
	ActiveValidatorCount int64   `json:"active_validator_count,omitempty"`
	ParticipationRate    float64 `json:"participation_rate,omitempty"`
}

// ValidatorsSummaryResponse is the response for dora://network/{name}/validators.
type ValidatorsSummaryResponse struct {
	Network     string             `json:"network"`
	DoraURL     string             `json:"dora_url"`
	Summary     *ValidatorsSummary `json:"summary,omitempty"`
	Error       string             `json:"error,omitempty"`
	ExploreLink string             `json:"explore_link"`
}

// ValidatorsSummary contains validator summary data.
type ValidatorsSummary struct {
	TotalCount   int64 `json:"total_count,omitempty"`
	ActiveCount  int64 `json:"active_count,omitempty"`
	PendingCount int64 `json:"pending_count,omitempty"`
	ExitedCount  int64 `json:"exited_count,omitempty"`
}

// RegisterDoraResources registers Dora MCP resources with the registry.
func RegisterDoraResources(
	log logrus.FieldLogger,
	reg plugin.ResourceRegistry,
	client resource.CartographoorClient,
) {
	log = log.WithField("resource", "dora")

	// dora://networks - List all networks with Dora explorers
	reg.RegisterStatic(types.StaticResource{
		Resource: mcp.NewResource(
			"dora://networks",
			"Dora Networks",
			mcp.WithResourceDescription("List all Ethereum networks with Dora beacon chain explorers"),
			mcp.WithMIMEType("application/json"),
			mcp.WithAnnotations([]mcp.Role{mcp.RoleAssistant}, 0.7),
		),
		Handler: createNetworksListHandler(client),
	})

	// dora://network/{name} - Network overview
	networkTemplate := mcp.NewResourceTemplate(
		"dora://network/{name}",
		"Dora Network Overview",
		mcp.WithTemplateDescription("Get network overview from Dora including current epoch, slot, and validator info"),
		mcp.WithTemplateMIMEType("application/json"),
		mcp.WithTemplateAnnotations([]mcp.Role{mcp.RoleAssistant}, 0.6),
	)

	reg.RegisterTemplate(types.TemplateResource{
		Template: networkTemplate,
		Pattern:  regexp.MustCompile(`^dora://network/([^/]+)$`),
		Handler:  createNetworkOverviewHandler(log, client),
	})

	// dora://network/{name}/validators - Validator summary
	validatorsTemplate := mcp.NewResourceTemplate(
		"dora://network/{name}/validators",
		"Dora Validators Summary",
		mcp.WithTemplateDescription("Get validator summary for a network from Dora"),
		mcp.WithTemplateMIMEType("application/json"),
		mcp.WithTemplateAnnotations([]mcp.Role{mcp.RoleAssistant}, 0.5),
	)

	reg.RegisterTemplate(types.TemplateResource{
		Template: validatorsTemplate,
		Pattern:  regexp.MustCompile(`^dora://network/([^/]+)/validators$`),
		Handler:  createValidatorsSummaryHandler(log, client),
	})

	log.Debug("Registered Dora resources")
}

// createNetworksListHandler creates a handler for dora://networks.
func createNetworksListHandler(client resource.CartographoorClient) types.ReadHandler {
	return func(_ context.Context, _ string) (string, error) {
		networks := client.GetActiveNetworks()

		response := &NetworksListResponse{
			Description: "Ethereum networks with Dora beacon chain explorers. Use dora://network/{name} for details.",
			Networks:    make([]NetworkInfo, 0, len(networks)),
			Usage:       "Access dora://network/{name} for network overview, dora://network/{name}/validators for validator info",
		}

		// Collect networks with Dora URLs
		for name, network := range networks {
			if network.ServiceURLs != nil && network.ServiceURLs.Dora != "" {
				response.Networks = append(response.Networks, NetworkInfo{
					Name:    name,
					DoraURL: network.ServiceURLs.Dora,
					Status:  network.Status,
				})
			}
		}

		// Sort by name for consistent output
		sort.Slice(response.Networks, func(i, j int) bool {
			return response.Networks[i].Name < response.Networks[j].Name
		})

		data, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshaling networks list: %w", err)
		}

		return string(data), nil
	}
}

// createNetworkOverviewHandler creates a handler for dora://network/{name}.
func createNetworkOverviewHandler(
	log logrus.FieldLogger,
	client resource.CartographoorClient,
) types.ReadHandler {
	return func(ctx context.Context, uri string) (string, error) {
		// Extract network name from URI
		networkName := extractNetworkName(uri, `^dora://network/([^/]+)$`)
		if networkName == "" {
			return "", fmt.Errorf("invalid network URI: %s", uri)
		}

		network, found := client.GetNetwork(networkName)
		if !found {
			return "", fmt.Errorf("network %q not found", networkName)
		}

		doraURL := ""
		if network.ServiceURLs != nil {
			doraURL = network.ServiceURLs.Dora
		}

		if doraURL == "" {
			return "", fmt.Errorf("network %q does not have a Dora explorer configured", networkName)
		}

		response := &NetworkOverviewResponse{
			Network: networkName,
			DoraURL: doraURL,
			Links: &NetworkLinks{
				Validators: doraURL + "/validators",
				Epochs:     doraURL + "/epochs",
				Slots:      doraURL + "/slots",
			},
		}

		// Fetch overview from Dora API
		overview, err := fetchDoraOverview(ctx, doraURL)
		if err != nil {
			log.WithError(err).WithField("network", networkName).Debug("Failed to fetch Dora overview")
			response.Error = fmt.Sprintf("Failed to fetch overview: %v", err)
		} else {
			response.Overview = overview
		}

		data, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshaling network overview: %w", err)
		}

		return string(data), nil
	}
}

// createValidatorsSummaryHandler creates a handler for dora://network/{name}/validators.
func createValidatorsSummaryHandler(
	log logrus.FieldLogger,
	client resource.CartographoorClient,
) types.ReadHandler {
	return func(ctx context.Context, uri string) (string, error) {
		// Extract network name from URI
		networkName := extractNetworkName(uri, `^dora://network/([^/]+)/validators$`)
		if networkName == "" {
			return "", fmt.Errorf("invalid validators URI: %s", uri)
		}

		network, found := client.GetNetwork(networkName)
		if !found {
			return "", fmt.Errorf("network %q not found", networkName)
		}

		doraURL := ""
		if network.ServiceURLs != nil {
			doraURL = network.ServiceURLs.Dora
		}

		if doraURL == "" {
			return "", fmt.Errorf("network %q does not have a Dora explorer configured", networkName)
		}

		response := &ValidatorsSummaryResponse{
			Network:     networkName,
			DoraURL:     doraURL,
			ExploreLink: doraURL + "/validators",
		}

		// Fetch validator summary from Dora API
		summary, err := fetchValidatorsSummary(ctx, doraURL)
		if err != nil {
			log.WithError(err).WithField("network", networkName).Debug("Failed to fetch validators summary")
			response.Error = fmt.Sprintf("Failed to fetch validators: %v", err)
		} else {
			response.Summary = summary
		}

		data, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshaling validators summary: %w", err)
		}

		return string(data), nil
	}
}

// extractNetworkName extracts the network name from a URI using the given pattern.
func extractNetworkName(uri, pattern string) string {
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(uri)

	if len(matches) < 2 {
		return ""
	}

	return matches[1]
}

// fetchDoraOverview fetches network overview from Dora API.
func fetchDoraOverview(ctx context.Context, baseURL string) (*DoraOverview, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/epoch/head", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching overview: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp struct {
		Data struct {
			Epoch         int64   `json:"epoch"`
			Finalized     bool    `json:"finalized"`
			Participation float64 `json:"globalparticipationrate"`
			ValidatorInfo struct {
				TotalCount  int64 `json:"total"`
				ActiveCount int64 `json:"active"`
			} `json:"validatorinfo,omitempty"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	// Calculate current slot (rough estimate: epoch * 32)
	currentSlot := apiResp.Data.Epoch * 32

	overview := &DoraOverview{
		CurrentEpoch:         apiResp.Data.Epoch,
		CurrentSlot:          currentSlot,
		ParticipationRate:    apiResp.Data.Participation,
		ActiveValidatorCount: apiResp.Data.ValidatorInfo.ActiveCount,
	}

	if apiResp.Data.Finalized {
		overview.FinalizedEpoch = apiResp.Data.Epoch
	}

	return overview, nil
}

// fetchValidatorsSummary fetches validator summary from Dora API.
func fetchValidatorsSummary(ctx context.Context, baseURL string) (*ValidatorsSummary, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/epoch/head", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching validators: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var apiResp struct {
		Data struct {
			ValidatorInfo struct {
				TotalCount   int64 `json:"total"`
				ActiveCount  int64 `json:"active"`
				PendingCount int64 `json:"pending"`
				ExitedCount  int64 `json:"exited"`
			} `json:"validatorinfo,omitempty"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &ValidatorsSummary{
		TotalCount:   apiResp.Data.ValidatorInfo.TotalCount,
		ActiveCount:  apiResp.Data.ValidatorInfo.ActiveCount,
		PendingCount: apiResp.Data.ValidatorInfo.PendingCount,
		ExitedCount:  apiResp.Data.ValidatorInfo.ExitedCount,
	}, nil
}
