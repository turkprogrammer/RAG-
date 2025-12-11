package unit

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"rag-system/src/domain"
	"rag-system/src/infrastructure"
)

// TestIndexingPerformance проверяет производительность индексации больших документов
func TestIndexingPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Пропускаем тест производительности в коротком режиме")
	}

	dbPath := "/tmp/test_perf_indexing.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	// Создаем документ размером ~1MB
	largeContent := strings.Repeat("Это тестовый текст для проверки производительности индексации. ", 20000)

	largeDoc := domain.Document{
		ID:      "perf-large-doc",
		Title:   "Большой документ для теста производительности",
		Content: largeContent,
	}

	start := time.Now()
	err = repo.SaveDocument(largeDoc)
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.Less(t, duration, 10*time.Second, "Индексация 1MB документа должна занимать менее 10 секунд")

	t.Logf("Индексация 1MB документа заняла: %v", duration)
}

// TestIndexingMultipleDocuments проверяет производительность индексации множества документов
func TestIndexingMultipleDocuments(t *testing.T) {
	if testing.Short() {
		t.Skip("Пропускаем тест производительности в коротком режиме")
	}

	dbPath := "/tmp/test_perf_multi.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	// Создаем 100 документов среднего размера
	numDocs := 100
	docSize := 5000 // ~5KB каждый

	start := time.Now()
	for i := 0; i < numDocs; i++ {
		content := strings.Repeat("Содержимое документа для теста производительности. ", docSize/50)
		doc := domain.Document{
			ID:      fmt.Sprintf("perf-doc-%d", i),
			Title:   fmt.Sprintf("Документ %d", i),
			Content: content,
		}
		err := repo.SaveDocument(doc)
		assert.NoError(t, err)
	}
	duration := time.Since(start)

	avgTime := duration / time.Duration(numDocs)
	assert.Less(t, avgTime, 100*time.Millisecond, "Среднее время индексации одного документа должно быть менее 100мс")

	t.Logf("Индексация %d документов заняла: %v (среднее: %v на документ)", numDocs, duration, avgTime)
}

// TestSearchPerformance проверяет производительность поиска в большой БД
func TestSearchPerformance(t *testing.T) {
	if testing.Short() {
		t.Skip("Пропускаем тест производительности в коротком режиме")
	}

	dbPath := "/tmp/test_perf_search.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	// Создаем 50 документов для поиска
	numDocs := 50
	for i := 0; i < numDocs; i++ {
		content := fmt.Sprintf("Документ номер %d содержит информацию о тестировании производительности системы поиска.", i)
		doc := domain.Document{
			ID:      fmt.Sprintf("search-doc-%d", i),
			Title:   fmt.Sprintf("Документ %d", i),
			Content: content,
		}
		err := repo.SaveDocument(doc)
		assert.NoError(t, err)
	}

	// Выполняем несколько поисковых запросов
	queries := []string{"производительности", "системы", "поиска", "документ"}

	for _, query := range queries {
		start := time.Now()
		chunks, err := repo.FindRelevantChunks(query, 10, 0.0)
		duration := time.Since(start)

		assert.NoError(t, err)
		assert.Less(t, duration, 500*time.Millisecond, "Поиск должен выполняться менее чем за 500мс")

		t.Logf("Поиск '%s' нашел %d результатов за %v", query, len(chunks), duration)
	}
}

// TestSearchPerformanceLargeDB проверяет производительность поиска в очень большой БД
func TestSearchPerformanceLargeDB(t *testing.T) {
	if testing.Short() {
		t.Skip("Пропускаем тест производительности в коротком режиме")
	}

	dbPath := "/tmp/test_perf_large_search.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	// Создаем 200 документов для имитации большой БД
	numDocs := 200
	for i := 0; i < numDocs; i++ {
		content := strings.Repeat(fmt.Sprintf("Текст документа %d для проверки производительности поиска. ", i), 10)
		doc := domain.Document{
			ID:      fmt.Sprintf("large-doc-%d", i),
			Title:   fmt.Sprintf("Документ %d", i),
			Content: content,
		}
		err := repo.SaveDocument(doc)
		assert.NoError(t, err)
	}

	// Выполняем поиск
	start := time.Now()
	chunks, err := repo.FindRelevantChunks("производительности", 20, 0.0)
	duration := time.Since(start)

	assert.NoError(t, err)
	assert.Less(t, duration, 1*time.Second, "Поиск в большой БД должен выполняться менее чем за 1 секунду")

	t.Logf("Поиск в БД с %d документами нашел %d результатов за %v", numDocs, len(chunks), duration)
}

// TestConcurrentIndexing проверяет производительность параллельной индексации
func TestConcurrentIndexing(t *testing.T) {
	if testing.Short() {
		t.Skip("Пропускаем тест производительности в коротком режиме")
	}

	dbPath := "/tmp/test_perf_concurrent.db"
	os.Remove(dbPath)

	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	// Индексируем 20 документов параллельно
	numDocs := 20
	done := make(chan error, numDocs)

	start := time.Now()
	for i := 0; i < numDocs; i++ {
		go func(id int) {
			doc := domain.Document{
				ID:      fmt.Sprintf("concurrent-doc-%d", id),
				Title:   fmt.Sprintf("Документ %d", id),
				Content: fmt.Sprintf("Содержимое документа %d для параллельной индексации.", id),
			}
			done <- repo.SaveDocument(doc)
		}(i)
	}

	// Ждем завершения всех горутин
	for i := 0; i < numDocs; i++ {
		err := <-done
		assert.NoError(t, err)
	}
	duration := time.Since(start)

	avgTime := duration / time.Duration(numDocs)
	t.Logf("Параллельная индексация %d документов заняла: %v (среднее: %v на документ)", numDocs, duration, avgTime)

	// Проверяем, что все документы сохранены
	allChunks, err := repo.FindRelevantChunks("", 1000, 0.0)
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, len(allChunks), numDocs, "Все документы должны быть сохранены")
}
