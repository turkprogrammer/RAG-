package unit

import (
	"github.com/stretchr/testify/assert"
	"rag-system/src/domain"
	"testing"
	"time"
)

func TestDocumentCreation(t *testing.T) {
	doc := domain.Document{
		ID:        "test-id",
		Title:     "Тестовый документ",
		Content:   "Содержимое тестового документа",
		CreatedAt: time.Now(),
	}

	assert.Equal(t, "test-id", doc.ID)
	assert.Equal(t, "Тестовый документ", doc.Title)
	assert.Equal(t, "Содержимое тестового документа", doc.Content)
	assert.NotZero(t, doc.CreatedAt)
}

func TestChunkCreation(t *testing.T) {
	chunk := domain.Chunk{
		ID:         "chunk-id",
		DocumentID: "doc-id",
		Content:    "Содержимое фрагмента",
		Similarity: 0.8,
	}

	assert.Equal(t, "chunk-id", chunk.ID)
	assert.Equal(t, "doc-id", chunk.DocumentID)
	assert.Equal(t, "Содержимое фрагмента", chunk.Content)
	assert.Equal(t, 0.8, chunk.Similarity)
}

func TestSearchRequestCreation(t *testing.T) {
	req := domain.SearchRequest{
		Query:     "поисковый запрос",
		Limit:     10,
		Threshold: 0.5,
	}

	assert.Equal(t, "поисковый запрос", req.Query)
	assert.Equal(t, 10, req.Limit)
	assert.Equal(t, 0.5, req.Threshold)
}

func TestSearchResultCreation(t *testing.T) {
	chunk := domain.Chunk{
		ID:         "chunk-id",
		DocumentID: "doc-id",
		Content:    "Содержимое фрагмента",
		Similarity: 0.8,
	}

	result := domain.SearchResult{
		Chunks: []domain.Chunk{chunk},
		Query:  "поисковый запрос",
	}

	assert.Equal(t, "поисковый запрос", result.Query)
	assert.Len(t, result.Chunks, 1)
	assert.Equal(t, "chunk-id", result.Chunks[0].ID)
}
