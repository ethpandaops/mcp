// Package embedding provides text embedding capabilities.
package embedding

import (
	"fmt"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

// Embedder provides text embedding capabilities.
type Embedder interface {
	// Embed returns the L2-normalized embedding vector for a single text string.
	Embed(text string) ([]float32, error)

	// EmbedBatch returns L2-normalized embedding vectors for multiple texts.
	EmbedBatch(texts []string) ([][]float32, error)

	// Close releases resources held by the embedder.
	Close() error
}

// LocalEmbedder provides text embedding using hugot's pure Go ONNX backend.
type LocalEmbedder struct {
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
}

// Compile-time interface check.
var _ Embedder = (*LocalEmbedder)(nil)

// NewLocal creates a new LocalEmbedder with the given ONNX model directory path.
func NewLocal(modelPath string) (*LocalEmbedder, error) {
	if modelPath == "" {
		return nil, fmt.Errorf("model path is required")
	}

	session, err := hugot.NewGoSession()
	if err != nil {
		return nil, fmt.Errorf("creating hugot session: %w", err)
	}

	config := hugot.FeatureExtractionConfig{
		ModelPath:    modelPath,
		Name:         "embedder",
		OnnxFilename: "model.onnx",
		Options: []hugot.FeatureExtractionOption{
			pipelines.WithNormalization(),
		},
	}

	pipeline, err := hugot.NewPipeline(session, config)
	if err != nil {
		_ = session.Destroy()
		return nil, fmt.Errorf("creating embedding pipeline from %s: %w", modelPath, err)
	}

	return &LocalEmbedder{session: session, pipeline: pipeline}, nil
}

// Embed returns the L2-normalized embedding vector for a single text string.
func (e *LocalEmbedder) Embed(text string) ([]float32, error) {
	result, err := e.pipeline.RunPipeline([]string{text})
	if err != nil {
		return nil, fmt.Errorf("embedding text: %w", err)
	}

	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return result.Embeddings[0], nil
}

// EmbedBatch returns L2-normalized embedding vectors for multiple texts.
func (e *LocalEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	result, err := e.pipeline.RunPipeline(texts)
	if err != nil {
		return nil, fmt.Errorf("embedding batch: %w", err)
	}

	return result.Embeddings, nil
}

// Close releases resources held by the embedder.
func (e *LocalEmbedder) Close() error {
	if e.session != nil {
		return e.session.Destroy()
	}

	return nil
}
