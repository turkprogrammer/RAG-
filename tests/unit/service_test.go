package unit

import (
	"github.com/stretchr/testify/assert"
	"rag-system/src/domain"
	"rag-system/tests/mocks"
	"testing"
)

func TestMockRepository(t *testing.T) {
	mockRepo := mocks.NewMockDocumentRepository()

	// Проверим, что mock репозиторий работает
	doc := domain.Document{
		ID:      "test-doc",
		Title:   "Тестовый документ",
		Content: "Содержимое тестового документа для проверки функциональности",
	}

	err := mockRepo.SaveDocument(doc)
	assert.NoError(t, err)

	chunks, err := mockRepo.FindRelevantChunks("тест", 10, 0.0)
	assert.NoError(t, err)
	assert.NotEmpty(t, chunks)

	docs, err := mockRepo.GetAllDocuments()
	assert.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "test-doc", docs[0].ID)
}
