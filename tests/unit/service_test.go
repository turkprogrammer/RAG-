package unit

import (
	"github.com/stretchr/testify/assert"
	"rag-system/src/domain"
	"rag-system/tests/mocks"
	"testing"
)

func TestMockRepository(t *testing.T) {
	mockRepo := mocks.NewMockDocumentRepository()

	// Сохраняем документ
	doc := domain.Document{
		ID:      "test-doc",
		Title:   "Тестовый документ",
		Content: "Содержимое тестового документа для проверки функциональности системы RAG.",
	}

	err := mockRepo.SaveDocument(doc)
	assert.NoError(t, err)

	// Проверяем, что документ сохранен
	docs, err := mockRepo.GetAllDocuments()
	assert.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "test-doc", docs[0].ID)

	// Проверяем поиск - должны найти фрагменты с высоким similarity
	chunks, err := mockRepo.FindRelevantChunks("тестовый", 10, 0.0)
	assert.NoError(t, err)
	assert.NotEmpty(t, chunks, "Должны быть найдены фрагменты по запросу 'тестовый'")

	// Проверяем, что similarity вычислен корректно
	for _, chunk := range chunks {
		assert.GreaterOrEqual(t, chunk.Similarity, 0.0)
		assert.LessOrEqual(t, chunk.Similarity, 1.0)
	}

	// Проверяем фильтрацию по threshold
	_, err = mockRepo.FindRelevantChunks("тестовый", 10, 0.8)
	assert.NoError(t, err)
	// Может быть пусто, если similarity < 0.8, это нормально

	// Проверяем поиск по несуществующему слову
	_, err = mockRepo.FindRelevantChunks("несуществующееслово12345", 10, 0.0)
	assert.NoError(t, err)
	// Может быть пусто или с низким similarity

	// Проверяем лимит
	limitedChunks, err := mockRepo.FindRelevantChunks("", 2, 0.0)
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(limitedChunks), 2, "Лимит должен работать")
}
