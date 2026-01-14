// Package embedding provides text embedding capabilities using local GGUF models.
package embedding

import (
	"fmt"

	"github.com/kelindar/search"
)

const (
	// DefaultGPULayers is the default number of GPU layers (0 = CPU only).
	DefaultGPULayers = 0
)

// Embedder provides text embedding capabilities using llama.cpp via kelindar/search.
type Embedder struct {
	vectorizer *search.Vectorizer
}

// New creates a new Embedder with the given GGUF model file path.
// Set gpuLayers > 0 to enable GPU acceleration (requires Vulkan).
func New(modelPath string, gpuLayers int) (*Embedder, error) {
	if modelPath == "" {
		return nil, fmt.Errorf("model path is required")
	}

	vectorizer, err := search.NewVectorizer(modelPath, gpuLayers)
	if err != nil {
		return nil, fmt.Errorf("initializing vectorizer from %s: %w", modelPath, err)
	}

	return &Embedder{vectorizer: vectorizer}, nil
}

// Embed returns the embedding vector for a single text string.
func (e *Embedder) Embed(text string) ([]float32, error) {
	return e.vectorizer.EmbedText(text)
}

// Close releases resources held by the embedder.
func (e *Embedder) Close() error {
	return e.vectorizer.Close()
}
