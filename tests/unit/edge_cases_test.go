package unit

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"rag-system/src/domain"
	"rag-system/src/infrastructure"
)

// TestEmptyDocument проверяет обработку пустых документов
func TestEmptyDocument(t *testing.T) {
	dbPath := "/tmp/test_empty_doc.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	// Пытаемся сохранить документ с пустым содержимым
	emptyDoc := domain.Document{
		ID:      "empty-doc",
		Title:   "Пустой документ",
		Content: "",
	}

	err = repo.SaveDocument(emptyDoc)
	assert.NoError(t, err, "Пустой документ должен сохраняться без ошибки")

	// Проверяем, что документ сохранен
	docs, err := repo.GetAllDocuments()
	assert.NoError(t, err)
	assert.Len(t, docs, 1)

	// Проверяем поиск - должен вернуть пустой результат или документ
	_, err = repo.FindRelevantChunks("любой запрос", 10, 0.0)
	assert.NoError(t, err)
	// Пустой документ может не дать результатов поиска, это нормально
}

// TestVeryLargeDocument проверяет обработку очень больших документов
func TestVeryLargeDocument(t *testing.T) {
	dbPath := "/tmp/test_large_doc.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	// Создаем большой документ (100KB текста)
	largeContent := make([]byte, 100*1024)
	for i := range largeContent {
		largeContent[i] = byte('A' + (i % 26))
		if i%100 == 0 {
			largeContent[i] = ' '
		}
	}

	largeDoc := domain.Document{
		ID:      "large-doc",
		Title:   "Очень большой документ",
		Content: string(largeContent),
	}

	start := time.Now()
	err = repo.SaveDocument(largeDoc)
	duration := time.Since(start)

	assert.NoError(t, err, "Большой документ должен сохраняться")
	assert.Less(t, duration, 5*time.Second, "Сохранение большого документа не должно занимать слишком много времени")

	// Проверяем, что документ разбит на чанки
	chunks, err := repo.FindRelevantChunks("", 100, 0.0)
	assert.NoError(t, err)
	assert.Greater(t, len(chunks), 0, "Большой документ должен быть разбит на чанки")
}

// TestSpecialCharactersInQuery проверяет обработку специальных символов в запросах
func TestSpecialCharactersInQuery(t *testing.T) {
	dbPath := "/tmp/test_special_chars.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	// Сохраняем документ с обычным содержимым
	doc := domain.Document{
		ID:      "special-doc",
		Title:   "Документ для теста специальных символов",
		Content: "Этот документ содержит обычный текст для проверки поиска.",
	}

	err = repo.SaveDocument(doc)
	assert.NoError(t, err)

	// Тестируем различные специальные символы в запросах
	specialQueries := []string{
		"'; DROP TABLE chunks; --",      // SQL инъекция
		"<script>alert('xss')</script>", // XSS попытка
		"\"quoted\"",                    // Кавычки
		"\\backslash\\",                 // Обратные слеши
		"%wildcard%",                    // SQL wildcards
		"тест\nс\nпереносами",           // Переносы строк
		"тест\tс\tтабуляциями",          // Табуляции
		"тест с множеством    пробелов", // Множественные пробелы
	}

	for _, query := range specialQueries {
		chunks, err := repo.FindRelevantChunks(query, 10, 0.0)
		assert.NoError(t, err, "Поиск с запросом '%s' не должен вызывать ошибку", query)
		// Результаты могут быть пустыми, но ошибок быть не должно
		_ = chunks
	}
}

// TestSpecialCharactersInContent проверяет обработку специальных символов в содержимом
func TestSpecialCharactersInContent(t *testing.T) {
	dbPath := "/tmp/test_special_content.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	// Документ с различными специальными символами
	specialDoc := domain.Document{
		ID:    "special-content-doc",
		Title: "Документ со специальными символами",
		Content: "Текст с кавычками \"двойными\" и 'одинарными'.\n" +
			"Текст с переносами строки.\n" +
			"Текст с табуляцией\tи пробелами.\n" +
			"Символы: !@#$%^&*()_+-=[]{}|;':\",./<>?",
	}

	err = repo.SaveDocument(specialDoc)
	assert.NoError(t, err, "Документ со специальными символами должен сохраняться")

	// Проверяем поиск
	chunks, err := repo.FindRelevantChunks("кавычками", 10, 0.0)
	assert.NoError(t, err)
	// Может найти или не найти, но ошибок быть не должно
	_ = chunks
}

// TestUnicodeCharacters проверяет обработку Unicode символов
func TestUnicodeCharacters(t *testing.T) {
	dbPath := "/tmp/test_unicode.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	// Документ с различными Unicode символами
	unicodeDoc := domain.Document{
		ID:    "unicode-doc",
		Title: "Документ с Unicode",
		Content: "Русский текст: Привет мир!\n" +
			"English text: Hello world!\n" +
			"中文: 你好世界\n" +
			"日本語: こんにちは\n" +
			"Emoji: 🚀 📚 💻 🌍",
	}

	err = repo.SaveDocument(unicodeDoc)
	assert.NoError(t, err, "Документ с Unicode должен сохраняться")

	// Проверяем поиск на разных языках
	queries := []string{"Привет", "Hello", "你好", "こんにちは"}
	for _, query := range queries {
		chunks, err := repo.FindRelevantChunks(query, 10, 0.0)
		assert.NoError(t, err, "Поиск Unicode запроса '%s' не должен вызывать ошибку", query)
		_ = chunks
	}
}

// TestEmptyQuery проверяет обработку пустых запросов
func TestEmptyQuery(t *testing.T) {
	dbPath := "/tmp/test_empty_query.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	doc := domain.Document{
		ID:      "test-doc",
		Title:   "Тестовый документ",
		Content: "Содержимое документа для тестирования.",
	}

	err = repo.SaveDocument(doc)
	assert.NoError(t, err)

	// Пустой запрос должен вернуть все фрагменты (или ограниченное количество)
	chunks, err := repo.FindRelevantChunks("", 10, 0.0)
	assert.NoError(t, err)
	assert.LessOrEqual(t, len(chunks), 10, "Пустой запрос должен учитывать лимит")
}

// TestNegativeLimit проверяет обработку отрицательных и нулевых лимитов
func TestNegativeLimit(t *testing.T) {
	dbPath := "/tmp/test_negative_limit.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	doc := domain.Document{
		ID:      "test-doc",
		Title:   "Тестовый документ",
		Content: "Содержимое документа.",
	}

	err = repo.SaveDocument(doc)
	assert.NoError(t, err)

	// Отрицательный лимит
	chunks, err := repo.FindRelevantChunks("документ", -1, 0.0)
	assert.NoError(t, err)
	// Поведение может варьироваться, но ошибок быть не должно
	_ = chunks

	// Нулевой лимит
	chunks2, err := repo.FindRelevantChunks("документ", 0, 0.0)
	assert.NoError(t, err)
	// Может вернуть пустой результат или все результаты
	_ = chunks2
}

// TestHighThreshold проверяет работу с высоким threshold
func TestHighThreshold(t *testing.T) {
	dbPath := "/tmp/test_high_threshold.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	doc := domain.Document{
		ID:      "test-doc",
		Title:   "Тестовый документ",
		Content: "Это документ для тестирования системы поиска.",
	}

	err = repo.SaveDocument(doc)
	assert.NoError(t, err)

	// Высокий threshold должен фильтровать результаты
	highThresholdChunks, err := repo.FindRelevantChunks("документ", 10, 0.9)
	assert.NoError(t, err)
	// Может быть пусто, если similarity < 0.9

	// Низкий threshold должен вернуть больше результатов
	lowThresholdChunks, err := repo.FindRelevantChunks("документ", 10, 0.1)
	assert.NoError(t, err)
	// Должно быть больше или равно результатов с высоким threshold
	assert.GreaterOrEqual(t, len(lowThresholdChunks), len(highThresholdChunks))
}
