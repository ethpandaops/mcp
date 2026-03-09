package searchruntime

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"

	"github.com/ethpandaops/mcp/pkg/config"
	"github.com/ethpandaops/mcp/pkg/embedding"
	"github.com/ethpandaops/mcp/pkg/extension"
	"github.com/ethpandaops/mcp/pkg/resource"
	"github.com/ethpandaops/mcp/runbooks"
)

type Runtime struct {
	ExampleIndex    *resource.ExampleIndex
	RunbookRegistry *runbooks.Registry
	RunbookIndex    *resource.RunbookIndex
	embedder        *embedding.Embedder
}

func Build(
	log logrus.FieldLogger,
	cfg config.SemanticSearchConfig,
	extensionRegistry *extension.Registry,
) (*Runtime, error) {
	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("semantic_search.model_path is required")
	}

	if _, err := os.Stat(cfg.ModelPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("embedding model not found at %s (run 'make download-models' to fetch it)", cfg.ModelPath)
	}

	embedder, err := embedding.New(cfg.ModelPath, cfg.GPULayers)
	if err != nil {
		return nil, fmt.Errorf("creating embedder: %w", err)
	}

	runtime := &Runtime{embedder: embedder}

	exampleIndex, err := resource.NewExampleIndex(log, embedder, resource.GetQueryExamples(extensionRegistry))
	if err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("building example index: %w", err)
	}

	runtime.ExampleIndex = exampleIndex
	log.Info("Semantic search example index built")

	runbookReg, err := runbooks.NewRegistry(log)
	if err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("creating runbook registry: %w", err)
	}

	runtime.RunbookRegistry = runbookReg
	if runbookReg.Count() == 0 {
		log.Warn("No runbooks found, runbook search will be disabled")
		return runtime, nil
	}

	runbookIndex, err := resource.NewRunbookIndex(log, embedder, runbookReg.All())
	if err != nil {
		_ = runtime.Close()
		return nil, fmt.Errorf("building runbook index: %w", err)
	}

	runtime.RunbookIndex = runbookIndex
	log.Info("Semantic search runbook index built")

	return runtime, nil
}

func (r *Runtime) Close() error {
	if r == nil {
		return nil
	}

	if r.ExampleIndex != nil {
		return r.ExampleIndex.Close()
	}

	if r.embedder != nil {
		return r.embedder.Close()
	}

	return nil
}
