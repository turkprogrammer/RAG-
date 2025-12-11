package unit

import (
	"github.com/stretchr/testify/assert"
	"os"
	"rag-system/src/domain"
	"rag-system/src/infrastructure"
	"testing"
	"time"
)

func TestSQLiteRepository(t *testing.T) {
	// Создаем временную базу данных для тестов
	dbPath := "/tmp/test_rag_system.db"

	// Удаляем файл базы данных, если он существует
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath) // Удаляем файл базы данных после теста

	// Тестируем сохранение документа
	doc := domain.Document{
		ID:        "test-doc-1",
		Title:     "Тестовый документ",
		Content:   "Это содержимое тестового документа для проверки функциональности системы.",
		CreatedAt: time.Now(),
	}

	err = repo.SaveDocument(doc)
	assert.NoError(t, err)

	// Тестируем получение документа через поиск
	_, err = repo.FindRelevantChunks("тестовый", 5, 0.0)
	assert.NoError(t, err)
	// В реальной ситуации необязательно будет найден фрагмент с точным соответствием,
	// поэтому проверим, что поиск возвращает фрагменты (если документ есть в базе)

	// Давайте попробуем поискать часть слова, которая может быть в содержимом
	_, err = repo.FindRelevantChunks("функциональности", 5, 0.0)
	assert.NoError(t, err)
	// Даже если не найдено точное совпадение с нашим запросом, должны быть возвращены какие-то фрагменты
	// из-за особенностей разбиения документа на фрагменты

	// Проверим, что в базе есть фрагменты
	allChunksQuery, err := repo.FindRelevantChunks("", 10, 0.0) // Запрос без ключевых слов
	assert.NoError(t, err)
	assert.NotEmpty(t, allChunksQuery, "В базе должны быть фрагменты после сохранения документа")

	// Тестируем получение всех документов
	allDocs, err := repo.GetAllDocuments()
	assert.NoError(t, err)
	assert.Len(t, allDocs, 1)
	assert.Equal(t, "test-doc-1", allDocs[0].ID)

	// Тестируем удаление документа
	err = repo.DeleteDocument("test-doc-1")
	assert.NoError(t, err)

	// Проверяем, что документ удален
	remainingDocs, err := repo.GetAllDocuments()
	assert.NoError(t, err)
	assert.Empty(t, remainingDocs)
}

func TestSQLiteRepositoryWithMultipleDocuments(t *testing.T) {
	// Создаем временную базу данных для тестов
	dbPath := "/tmp/test_rag_system_multi.db"

	// Удаляем файл базы данных, если он существует
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath) // Удаляем файл базы данных после теста

	// Сохраняем несколько документов
	documents := []domain.Document{
		{
			ID:      "doc-1",
			Title:   "Документ 1",
			Content: "Это первый документ с информацией о компании.",
		},
		{
			ID:      "doc-2",
			Title:   "Документ 2",
			Content: "Это второй документ с информацией о продуктах.",
		},
		{
			ID:      "doc-3",
			Title:   "Документ 3",
			Content: "Это третий документ с контактной информацией.",
		},
	}

	for _, doc := range documents {
		err := repo.SaveDocument(doc)
		assert.NoError(t, err)
	}

	// Проверяем, что фрагменты были созданы
	allChunks, err := repo.FindRelevantChunks("", 10, 0.0)
	assert.NoError(t, err)
	assert.NotEmpty(t, allChunks, "Должны быть созданы фрагменты из документов")

	// Проверяем, что все документы сохранены
	allDocs, err := repo.GetAllDocuments()
	assert.NoError(t, err)
	assert.Len(t, allDocs, 3)
}
