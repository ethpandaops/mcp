package resource

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/xatu-mcp/pkg/grafana"
)

// datasourcesResponse is the JSON response for datasources://list.
type datasourcesResponse struct {
	Datasources []datasourceData `json:"datasources"`
	Count       int              `json:"count"`
}

type datasourceData struct {
	UID         string `json:"uid"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	TypeNorm    string `json:"type_normalized"`
	Description string `json:"description,omitempty"`
}

// datasourcesByTypeResponse is the JSON response for datasources://{type}.
type datasourcesByTypeResponse struct {
	Type        string           `json:"type"`
	Datasources []datasourceData `json:"datasources"`
	Count       int              `json:"count"`
}

// RegisterDatasourcesResources registers the datasources:// resources with the registry.
func RegisterDatasourcesResources(
	log logrus.FieldLogger,
	reg Registry,
	grafanaClient grafana.Client,
) {
	log = log.WithField("resource", "datasources")

	// Register static datasources://list resource
	reg.RegisterStatic(StaticResource{
		Resource: mcp.NewResource(
			"datasources://list",
			"Available Datasources",
			mcp.WithResourceDescription("List all Grafana datasources available for queries (ClickHouse, Prometheus, Loki)"),
			mcp.WithMIMEType("application/json"),
			mcp.WithAnnotations([]mcp.Role{mcp.RoleAssistant}, 0.8),
		),
		Handler: func(_ context.Context, _ string) (string, error) {
			datasources := grafanaClient.ListDatasources()
			data := make([]datasourceData, 0, len(datasources))

			for _, ds := range datasources {
				data = append(data, datasourceData{
					UID:         ds.UID,
					Name:        ds.Name,
					Type:        ds.Type,
					TypeNorm:    string(ds.TypeNorm),
					Description: ds.Description,
				})
			}

			response := datasourcesResponse{
				Datasources: data,
				Count:       len(data),
			}

			jsonData, err := json.MarshalIndent(response, "", "  ")
			if err != nil {
				return "", fmt.Errorf("marshaling datasources response: %w", err)
			}

			return string(jsonData), nil
		},
	})

	// Priority by datasource type (ClickHouse is primary, others are secondary)
	typePriority := map[grafana.DatasourceType]float64{
		grafana.DatasourceTypeClickHouse: 0.7,
		grafana.DatasourceTypePrometheus: 0.5,
		grafana.DatasourceTypeLoki:       0.5,
	}

	// Register static resources for each datasource type
	for _, dsType := range []grafana.DatasourceType{
		grafana.DatasourceTypeClickHouse,
		grafana.DatasourceTypePrometheus,
		grafana.DatasourceTypeLoki,
	} {
		dsTypeStr := string(dsType)

		reg.RegisterStatic(StaticResource{
			Resource: mcp.NewResource(
				fmt.Sprintf("datasources://%s", dsTypeStr),
				fmt.Sprintf("%s Datasources", capitalize(dsTypeStr)),
				mcp.WithResourceDescription(fmt.Sprintf("List available %s datasources", dsTypeStr)),
				mcp.WithMIMEType("application/json"),
				mcp.WithAnnotations([]mcp.Role{mcp.RoleAssistant}, typePriority[dsType]),
			),
			Handler: createDatasourceTypeHandler(grafanaClient, dsType),
		})
	}

	log.Debug("Registered datasources resources")
}

// createDatasourceTypeHandler creates a handler for listing datasources of a specific type.
func createDatasourceTypeHandler(
	grafanaClient grafana.Client,
	dsType grafana.DatasourceType,
) func(context.Context, string) (string, error) {
	return func(_ context.Context, _ string) (string, error) {
		datasources := grafanaClient.ListDatasourcesByType(dsType)
		data := make([]datasourceData, 0, len(datasources))

		for _, ds := range datasources {
			data = append(data, datasourceData{
				UID:         ds.UID,
				Name:        ds.Name,
				Type:        ds.Type,
				TypeNorm:    string(ds.TypeNorm),
				Description: ds.Description,
			})
		}

		response := datasourcesByTypeResponse{
			Type:        string(dsType),
			Datasources: data,
			Count:       len(data),
		}

		jsonData, err := json.MarshalIndent(response, "", "  ")
		if err != nil {
			return "", fmt.Errorf("marshaling datasources response: %w", err)
		}

		return string(jsonData), nil
	}
}

// capitalize returns the string with the first letter capitalized.
func capitalize(s string) string {
	if s == "" {
		return s
	}

	return string(s[0]-32) + s[1:]
}
