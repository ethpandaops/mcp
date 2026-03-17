// Package embedding provides text embedding capabilities.
package embedding

// Embedder provides text embedding capabilities.
type Embedder interface {
	// Embed returns the L2-normalized embedding vector for a single text string.
	Embed(text string) ([]float32, error)

	// EmbedBatch returns L2-normalized embedding vectors for multiple texts.
	EmbedBatch(texts []string) ([][]float32, error)

	// Close releases resources held by the embedder.
	Close() error
}
