package embedding

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

const remoteEmbedTimeout = 2 * time.Minute

// embedRequest is the request payload for the proxy /embed endpoint.
type embedRequest struct {
	Items []embedItem `json:"items"`
}

// embedItem is a single item to embed.
type embedItem struct {
	Hash string `json:"hash"`
	Text string `json:"text"`
}

// embedResponse is the response payload from the proxy /embed endpoint.
type embedResponse struct {
	Results []embedResult `json:"results"`
	Model   string        `json:"model"`
}

// embedResult is a single embedding result.
type embedResult struct {
	Hash   string    `json:"hash"`
	Vector []float32 `json:"vector"`
}

// RemoteEmbedder implements Embedder by calling the proxy's /embed endpoint.
type RemoteEmbedder struct {
	log        logrus.FieldLogger
	proxyURL   string
	httpClient *http.Client
	tokenFn    func() string
}

// Compile-time interface check.
var _ Embedder = (*RemoteEmbedder)(nil)

// NewRemote creates a new RemoteEmbedder that calls the proxy's /embed endpoint.
// tokenFn is called on each request to get the current auth token.
func NewRemote(log logrus.FieldLogger, proxyURL string, tokenFn func() string) *RemoteEmbedder {
	return &RemoteEmbedder{
		log:        log.WithField("component", "remote-embedder"),
		proxyURL:   proxyURL,
		httpClient: &http.Client{Timeout: remoteEmbedTimeout},
		tokenFn:    tokenFn,
	}
}

// Embed returns the L2-normalized embedding vector for a single text string.
func (e *RemoteEmbedder) Embed(text string) ([]float32, error) {
	vectors, err := e.EmbedBatch([]string{text})
	if err != nil {
		return nil, err
	}

	if len(vectors) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return vectors[0], nil
}

// EmbedBatch returns L2-normalized embedding vectors for multiple texts.
func (e *RemoteEmbedder) EmbedBatch(texts []string) ([][]float32, error) {
	items := make([]embedItem, len(texts))
	// Track all indices per hash to handle duplicate texts correctly.
	hashToIndices := make(map[string][]int, len(texts))

	for i, text := range texts {
		hash := sha256Hex(text)
		items[i] = embedItem{Hash: hash, Text: text}
		hashToIndices[hash] = append(hashToIndices[hash], i)
	}

	reqBody, err := json.Marshal(embedRequest{Items: items})
	if err != nil {
		return nil, fmt.Errorf("marshaling embed request: %w", err)
	}

	url := fmt.Sprintf("%s/embed", e.proxyURL)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("creating embed request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	if token := e.tokenFn(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling proxy embed endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)

		return nil, fmt.Errorf("proxy embed returned status %d: %s", resp.StatusCode, string(body))
	}

	var embedResp embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&embedResp); err != nil {
		return nil, fmt.Errorf("decoding embed response: %w", err)
	}

	// Map results back to input order by hash, assigning to all indices.
	vectors := make([][]float32, len(texts))
	for _, result := range embedResp.Results {
		indices, ok := hashToIndices[result.Hash]
		if !ok {
			continue
		}

		for _, idx := range indices {
			vectors[idx] = result.Vector
		}
	}

	for i, v := range vectors {
		if v == nil {
			return nil, fmt.Errorf("missing embedding for text at index %d", i)
		}
	}

	return vectors, nil
}

// Close is a no-op for the remote embedder.
func (e *RemoteEmbedder) Close() error {
	return nil
}

func sha256Hex(text string) string {
	h := sha256.Sum256([]byte(text))

	return fmt.Sprintf("%x", h)
}
