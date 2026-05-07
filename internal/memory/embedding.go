package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// EmbeddingClient generates vector embeddings for text.
type EmbeddingClient interface {
	// Embed returns a vector embedding for the given text.
	Embed(ctx context.Context, text string) ([]float64, error)
}

// OpenAIEmbeddingClient uses OpenAI's embedding API to generate vectors.
type OpenAIEmbeddingClient struct {
	apiKey    string
	model     string
	httpClient *http.Client
}

// NewOpenAIEmbeddingClient creates a new OpenAI embedding client.
func NewOpenAIEmbeddingClient(apiKey, model string) *OpenAIEmbeddingClient {
	return &OpenAIEmbeddingClient{
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Embed returns a vector embedding for the given text using OpenAI's API.
func (c *OpenAIEmbeddingClient) Embed(ctx context.Context, text string) ([]float64, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("embedding: openai api key not set")
	}
	if text == "" {
		return nil, fmt.Errorf("embedding: text is empty")
	}

	// Prepare the request to OpenAI's embedding API.
	reqBody := map[string]interface{}{
		"input": text,
		"model": c.model,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("embedding: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.openai.com/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embedding: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding: http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embedding: openai api error: %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("embedding: decode response: %w", err)
	}

	if len(result.Data) == 0 {
		return nil, fmt.Errorf("embedding: no embedding returned")
	}

	return result.Data[0].Embedding, nil
}
