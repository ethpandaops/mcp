package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/mcp/pkg/resource"
	"github.com/ethpandaops/mcp/runbooks"
)

const (
	SearchRunbooksToolName    = "search_runbooks"
	DefaultRunbookSearchLimit = 3
	MaxRunbookSearchLimit     = 5
	MinRunbookSimilarityScore = 0.25
)

const searchRunbooksDescription = `Search procedural runbooks for multi-step investigations.

Use this for HOW to approach a diagnosis (vs search_examples for query snippets).

Runbooks contain step-by-step procedures with MUST/SHOULD/MAY constraints following RFC 2119.

Examples:
- search_runbooks(query="network not finalizing") → finality investigation procedure
- search_runbooks(query="blocks arriving late") → block propagation diagnosis
- search_runbooks(query="slow clickhouse query") → query optimization guide`

// SearchRunbookResult represents a single runbook search result.
type SearchRunbookResult struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Tags            []string `json:"tags"`
	Prerequisites   []string `json:"prerequisites"`
	Content         string   `json:"content"`
	FilePath        string   `json:"file_path"`
	SimilarityScore float64  `json:"similarity_score"`
}

// SearchRunbooksResponse is the response from the search_runbooks tool.
type SearchRunbooksResponse struct {
	Query         string                 `json:"query"`
	TagFilter     string                 `json:"tag_filter,omitempty"`
	TotalMatches  int                    `json:"total_matches"`
	Results       []*SearchRunbookResult `json:"results"`
	AvailableTags []string               `json:"available_tags"`
}

type searchRunbooksHandler struct {
	log        logrus.FieldLogger
	index      *resource.RunbookIndex
	runbookReg *runbooks.Registry
}

// NewSearchRunbooksTool creates the search_runbooks MCP tool.
func NewSearchRunbooksTool(
	log logrus.FieldLogger,
	index *resource.RunbookIndex,
	runbookReg *runbooks.Registry,
) Definition {
	h := &searchRunbooksHandler{
		log:        log.WithField("tool", SearchRunbooksToolName),
		index:      index,
		runbookReg: runbookReg,
	}

	return Definition{
		Tool: mcp.Tool{
			Name:        SearchRunbooksToolName,
			Description: searchRunbooksDescription,
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "Search term or phrase to find semantically similar runbooks",
					},
					"tag": map[string]any{
						"type":        "string",
						"description": "Optional: filter to runbooks with a specific tag (e.g., 'finality', 'performance')",
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum results to return (default: 3, max: 5)",
						"minimum":     1,
						"maximum":     MaxRunbookSearchLimit,
					},
				},
				Required: []string{"query"},
			},
		},
		Handler: h.handle,
	}
}

func (h *searchRunbooksHandler) handle(_ context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	h.log.Debug("Handling search_runbooks request")

	query := request.GetString("query", "")
	if query == "" {
		return CallToolError(fmt.Errorf("query is required and cannot be empty")), nil
	}

	tagFilter := request.GetString("tag", "")

	limit := request.GetInt("limit", DefaultRunbookSearchLimit)
	if limit <= 0 {
		limit = DefaultRunbookSearchLimit
	}

	if limit > MaxRunbookSearchLimit {
		limit = MaxRunbookSearchLimit
	}

	// Get available tags for the response.
	availableTags := h.runbookReg.Tags()
	sort.Strings(availableTags)

	// Validate tag filter if provided.
	if tagFilter != "" && !slices.Contains(availableTags, tagFilter) {
		return CallToolError(fmt.Errorf(
			"unknown tag: %q. Available tags: %s",
			tagFilter,
			strings.Join(availableTags, ", "),
		)), nil
	}

	// Perform semantic search.
	results, err := h.index.Search(query, limit*2) // Fetch extra to account for filtering
	if err != nil {
		return CallToolError(fmt.Errorf("search failed: %w", err)), nil
	}

	// Filter by tag if specified.
	if tagFilter != "" {
		filtered := make([]resource.RunbookSearchResult, 0, len(results))
		for _, r := range results {
			if slices.Contains(r.Runbook.Tags, tagFilter) {
				filtered = append(filtered, r)
			}
		}

		results = filtered
	}

	// Apply limit after filtering.
	if len(results) > limit {
		results = results[:limit]
	}

	// Build response.
	searchResults := make([]*SearchRunbookResult, 0, len(results))

	for _, r := range results {
		if r.Score < MinRunbookSimilarityScore {
			continue
		}

		searchResults = append(searchResults, &SearchRunbookResult{
			Name:            r.Runbook.Name,
			Description:     r.Runbook.Description,
			Tags:            r.Runbook.Tags,
			Prerequisites:   r.Runbook.Prerequisites,
			Content:         r.Runbook.Content,
			FilePath:        r.Runbook.FilePath,
			SimilarityScore: r.Score,
		})
	}

	response := &SearchRunbooksResponse{
		Query:         query,
		TagFilter:     tagFilter,
		TotalMatches:  len(searchResults),
		Results:       searchResults,
		AvailableTags: availableTags,
	}

	data, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return CallToolError(fmt.Errorf("marshaling response: %w", err)), nil
	}

	h.log.WithFields(logrus.Fields{
		"query":   query,
		"matches": len(searchResults),
	}).Debug("Runbook search completed")

	return CallToolSuccess(string(data)), nil
}
