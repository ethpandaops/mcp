package embedding

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMockProxyEmbedServer creates a test server that mimics the proxy /embed endpoint.
// The handler receives a decoded embedRequest, and the caller controls the response
// via the returned channel or by providing a static handler function.
func newMockProxyEmbedServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return srv
}

func TestRemoteEmbedder_Embed(t *testing.T) {
	t.Parallel()

	fakeVector := []float32{0.1, 0.2, 0.3}

	srv := newMockProxyEmbedServer(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "/embed", r.URL.Path)

		var req embedRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Len(t, req.Items, 1)

		resp := embedResponse{
			Model: "test-model",
			Results: []embedResult{
				{Hash: req.Items[0].Hash, Vector: fakeVector},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	})

	embedder := NewRemote(logrus.New(), srv.URL, func() string { return "" })

	vec, err := embedder.Embed("hello world")
	require.NoError(t, err)
	assert.Equal(t, fakeVector, vec)
}

func TestRemoteEmbedder_EmbedBatch(t *testing.T) {
	t.Parallel()

	texts := []string{"alpha", "beta", "gamma"}
	fakeVectors := map[string][]float32{
		"alpha": {1.0, 0.0, 0.0},
		"beta":  {0.0, 1.0, 0.0},
		"gamma": {0.0, 0.0, 1.0},
	}

	srv := newMockProxyEmbedServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req embedRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Len(t, req.Items, 3)

		results := make([]embedResult, 0, len(req.Items))
		for _, item := range req.Items {
			results = append(results, embedResult{
				Hash:   item.Hash,
				Vector: fakeVectors[item.Text],
			})
		}

		resp := embedResponse{Model: "test-model", Results: results}

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	})

	embedder := NewRemote(logrus.New(), srv.URL, func() string { return "" })

	vectors, err := embedder.EmbedBatch(texts)
	require.NoError(t, err)
	require.Len(t, vectors, 3)

	// Verify vectors are returned in the same order as input texts.
	assert.Equal(t, fakeVectors["alpha"], vectors[0])
	assert.Equal(t, fakeVectors["beta"], vectors[1])
	assert.Equal(t, fakeVectors["gamma"], vectors[2])
}

func TestRemoteEmbedder_EmbedBatch_DuplicateTexts(t *testing.T) {
	t.Parallel()

	// Two identical texts should both get vectors assigned. This was a bug where
	// the hash-to-index mapping only tracked a single index per hash.
	texts := []string{"duplicate", "duplicate"}
	fakeVector := []float32{0.5, 0.5, 0.5}

	srv := newMockProxyEmbedServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req embedRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		// Both items have the same hash, but both are sent.
		require.Len(t, req.Items, 2)

		hash := sha256Hex("duplicate")
		// The proxy only needs to return one result per unique hash.
		resp := embedResponse{
			Model: "test-model",
			Results: []embedResult{
				{Hash: hash, Vector: fakeVector},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	})

	embedder := NewRemote(logrus.New(), srv.URL, func() string { return "" })

	vectors, err := embedder.EmbedBatch(texts)
	require.NoError(t, err)
	require.Len(t, vectors, 2)

	// Both indices must have received the vector.
	assert.Equal(t, fakeVector, vectors[0])
	assert.Equal(t, fakeVector, vectors[1])
}

func TestRemoteEmbedder_ServerError(t *testing.T) {
	t.Parallel()

	srv := newMockProxyEmbedServer(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	})

	embedder := NewRemote(logrus.New(), srv.URL, func() string { return "" })

	_, err := embedder.Embed("test")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "500")
}

func TestRemoteEmbedder_AuthHeader(t *testing.T) {
	t.Parallel()

	const expectedToken = "my-secret-token"
	tokenCalled := false

	srv := newMockProxyEmbedServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Verify the Authorization header is set correctly.
		authHeader := r.Header.Get("Authorization")
		assert.Equal(t, "Bearer "+expectedToken, authHeader)

		hash := fmt.Sprintf("%x", sha256.Sum256([]byte("test")))

		resp := embedResponse{
			Model: "test-model",
			Results: []embedResult{
				{Hash: hash, Vector: []float32{0.1, 0.2, 0.3}},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	})

	tokenFn := func() string {
		tokenCalled = true

		return expectedToken
	}

	embedder := NewRemote(logrus.New(), srv.URL, tokenFn)

	_, err := embedder.Embed("test")
	require.NoError(t, err)
	assert.True(t, tokenCalled, "token function should have been called")
}
