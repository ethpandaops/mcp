package resource

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"
)

// ToolLister provides access to registered tools.
type ToolLister interface {
	List() []mcp.Tool
}

// gettingStartedHeader contains the static workflow guidance.
const gettingStartedHeader = `# Xatu Getting Started Guide

## Quick Start Workflow

1. **Read datasources://clickhouse** to get the exact datasource UIDs you need
2. **Use search_examples tool** to find relevant query patterns for your task
3. **Check clickhouse://tables/{table}** for exact column names and partition keys
4. **Replace placeholders in examples**: Examples use ` + "`{network}`" + ` placeholder - replace with actual network (e.g., ` + "`mainnet`" + `, ` + "`sepolia`" + `)
5. **Execute with execute_python** using the adapted example

## ⚠️ CRITICAL: Cluster Rules

Xatu data is split across TWO clusters with DIFFERENT query syntax:

| Cluster | Contains | Table Syntax | Network Filter |
|---------|----------|--------------|----------------|
| **xatu** | Raw event data | ` + "`FROM table_name`" + ` | ` + "`WHERE meta_network_name = 'mainnet'`" + ` |
| **xatu-cbt** | Aggregated data (faster) | ` + "`FROM mainnet.table_name`" + ` | Database prefix IS the filter |

**Key rules:**
- Always check which cluster (` + "`cluster: xatu`" + ` or ` + "`cluster: xatu-cbt`" + `) an example uses
- The datasource UID determines which cluster you query - check datasources://clickhouse
- Always filter by partition column (usually ` + "`slot_start_date_time`" + `) to avoid timeouts

## Canonical vs Head Data

- **Canonical**: Finalized data - confirmed by consensus, no reorgs possible
- **Head**: Latest data - may be reorged, use for real-time monitoring
- Tables often have both variants (e.g., ` + "`fct_block_head`" + ` vs ` + "`fct_block_canonical`" + `)
- When analyzing historical data, prefer canonical tables
- When analyzing reorgs/forks, you MUST account for survivorship bias (orphaned blocks are excluded from canonical)
`

// gettingStartedFooter contains static tips.
const gettingStartedFooter = `
## Sessions & File Persistence

- Files written to ` + "`/workspace/`" + ` persist within a session
- Pass the ` + "`session_id`" + ` from tool responses to continue a session
- Sessions expire after inactivity - check the ` + "`ttl`" + ` in responses
- For important outputs, use ` + "`storage.upload()`" + ` immediately to get a permanent URL

## Tips

- Use search_examples("block") to find block-related query patterns
- Use search_examples("validator") to find validator-related patterns
- Avoid spamming stdout with too much text - keep output concise
- If the user asks for a chart or file, use storage.upload() to upload and return the URL
  - Note: If you are Claude Code, you may need to manually recite the URL to the user towards the end of your response to avoid it being cut off.
`

// RegisterGettingStartedResources registers the xatu://getting-started resource.
func RegisterGettingStartedResources(
	log logrus.FieldLogger,
	reg Registry,
	toolReg ToolLister,
) {
	log = log.WithField("resource", "getting_started")

	reg.RegisterStatic(StaticResource{
		Resource: mcp.Resource{
			URI:         "xatu://getting-started",
			Name:        "Xatu Getting Started Guide",
			Description: "Essential guide for querying Ethereum data with Xatu - read this first!",
			MIMEType:    "text/markdown",
		},
		Handler: createGettingStartedHandler(reg, toolReg),
	})

	log.Debug("Registered getting-started resource")
}

// createGettingStartedHandler creates a handler that dynamically builds content.
func createGettingStartedHandler(reg Registry, toolReg ToolLister) ReadHandler {
	return func(_ context.Context, _ string) (string, error) {
		var sb strings.Builder

		// Write header with workflow and critical requirements
		sb.WriteString(gettingStartedHeader)

		// Dynamically list tools
		sb.WriteString("## Available Tools\n\n")

		tools := toolReg.List()
		sort.Slice(tools, func(i, j int) bool {
			return tools[i].Name < tools[j].Name
		})

		for _, tool := range tools {
			// Get first line of description
			desc := tool.Description
			if idx := strings.Index(desc, "\n"); idx > 0 {
				desc = desc[:idx]
			}

			// Trim any leading emoji or special chars for cleaner output
			desc = strings.TrimSpace(desc)
			if strings.HasPrefix(desc, "⚠️") {
				// Skip warning lines, get next meaningful line
				lines := strings.Split(tool.Description, "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line != "" && !strings.HasPrefix(line, "⚠️") {
						desc = line
						break
					}
				}
			}

			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", tool.Name, desc))
		}

		// Dynamically list resources
		sb.WriteString("\n## Available Resources\n\n")

		// Static resources
		staticResources := reg.ListStatic()
		sort.Slice(staticResources, func(i, j int) bool {
			return staticResources[i].URI < staticResources[j].URI
		})

		for _, res := range staticResources {
			// Skip self-reference
			if res.URI == "xatu://getting-started" {
				continue
			}

			sb.WriteString(fmt.Sprintf("- `%s` - %s\n", res.URI, res.Name))
		}

		// Template resources
		templates := reg.ListTemplates()
		if len(templates) > 0 {
			sb.WriteString("\n**Templates:**\n")

			sort.Slice(templates, func(i, j int) bool {
				return templates[i].URITemplate.Raw() < templates[j].URITemplate.Raw()
			})

			for _, tmpl := range templates {
				sb.WriteString(fmt.Sprintf("- `%s` - %s\n", tmpl.URITemplate.Raw(), tmpl.Name))
			}
		}

		// Write footer with tips
		sb.WriteString(gettingStartedFooter)

		return sb.String(), nil
	}
}
