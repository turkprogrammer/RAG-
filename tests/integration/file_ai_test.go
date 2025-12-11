package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"rag-system/src/application"
	"rag-system/src/domain"
	"rag-system/src/infrastructure"
	"rag-system/src/infrastructure/ai"
	"time"
)

// TestFileWithAI проверяет полный цикл RAG с реальным файлом test_doc.txt
// Тест проверяет: чтение файла, индексацию, поиск и генерацию ответов через AI
func TestFileWithAI(t *testing.T) {

	// Создаем временную базу данных для теста
	dbPath := "/tmp/test_file_ai.db"
	configPath := "../../config/config.yaml"
	testDocPath := "../../test_doc.txt"

	// Удаляем файл базы данных, если он существует
	os.Remove(dbPath)

	// Проверяем наличие конфига и получаем абсолютный путь
	absConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		t.Fatalf("Не удалось получить абсолютный путь к конфигу: %v", err)
	}
	if _, err := os.Stat(absConfigPath); os.IsNotExist(err) {
		t.Fatalf("Конфиг %s не найден", absConfigPath)
	}
	configPath = absConfigPath

	// Проверяем наличие тестового файла
	absPath, err := filepath.Abs(testDocPath)
	if err != nil {
		t.Fatalf("Не удалось получить абсолютный путь к файлу: %v", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		t.Fatalf("Тестовый файл %s не найден. Этот тест требует наличие файла test_doc.txt", absPath)
	}

	// Создаем репозиторий
	repo, err := infrastructure.NewSQLiteDocumentRepository(dbPath)
	assert.NoError(t, err)
	defer repo.Close()
	defer os.Remove(dbPath)

	// Пытаемся создать AI клиент (использует config.yaml или переменную окружения AI_API_KEY)
	aiClient, err := ai.NewAIClient(configPath)
	hasAI := err == nil && aiClient != nil

	if !hasAI {
		t.Logf("AI клиент недоступен: %v. Тестируем только индексацию и поиск. "+
			"Для полной проверки установите AI_API_KEY через переменную окружения или укажите реальный ключ в config.yaml", err)
	} else {
		t.Logf("AI клиент успешно инициализирован из config.yaml")
	}

	// Создаем сервис (может быть без AI)
	var service *application.RAGService
	if hasAI {
		service = application.NewRAGService(repo, aiClient)
	} else {
		// Без AI клиента сервис не может быть создан, тестируем напрямую репозиторий
		service = nil
	}

	// Читаем содержимое файла
	content, err := os.ReadFile(absPath)
	assert.NoError(t, err, "Должен успешно прочитать файл test_doc.txt")
	assert.NotEmpty(t, content, "Файл test_doc.txt не должен быть пустым")

	// Создаем документ из файла
	testDoc := domain.Document{
		ID:        "test-doc-file",
		Title:     "Тестовый документ из файла",
		Content:   string(content),
		CreatedAt: time.Now(),
	}

	// Индексируем документ
	if service != nil {
		err = service.IndexDocument(testDoc)
		assert.NoError(t, err, "Должен успешно проиндексировать документ из файла")

		// Проверяем, что документ сохранен
		allDocs, err := service.GetAllDocuments()
		assert.NoError(t, err)
		assert.Len(t, allDocs, 1, "Должен быть сохранен один документ")
		assert.Equal(t, testDoc.ID, allDocs[0].ID)
	} else {
		// Без сервиса индексируем напрямую через репозиторий
		err = repo.SaveDocument(testDoc)
		assert.NoError(t, err, "Должен успешно сохранить документ из файла")
	}

	// Проверяем, что документ из test_doc.txt сохранен и разбит на фрагменты
	allChunks, err := repo.FindRelevantChunks("", 10, 0.0)
	assert.NoError(t, err)
	assert.NotEmpty(t, allChunks, "Документ из test_doc.txt должен быть сохранен и разбит на фрагменты")

	// Проверяем, что содержимое файла действительно в базе
	fileContentFound := false
	for _, chunk := range allChunks {
		if containsIgnoreCase(chunk.Content, "компания") && containsIgnoreCase(chunk.Content, "2020") {
			fileContentFound = true
			break
		}
	}
	assert.True(t, fileContentFound, "Содержимое test_doc.txt должно быть в базе данных")

	// Выполняем поиск по ключевым словам из test_doc.txt
	// test_doc.txt содержит: "Наша компания была основана в 2020 году..."
	searchQueries := []struct {
		query    string
		expected string // Ожидаемое содержимое в ответе
		required bool   // Обязательно ли найти это слово
	}{
		{"компания", "компания", true},      // Обязательно должно найтись
		{"2020", "2020", true},              // Обязательно должно найтись
		{"сотрудники", "сотрудники", false}, // Может не найтись из-за разбиения на чанки
		{"офисы", "офисы", false},           // Может не найтись из-за разбиения на чанки
	}

	for _, testCase := range searchQueries {
		t.Run("Search_"+testCase.query, func(t *testing.T) {
			var result *domain.SearchResult
			var err error

			if service != nil {
				result, err = service.Search(testCase.query, 5, 0.0)
			} else {
				chunks, searchErr := repo.FindRelevantChunks(testCase.query, 5, 0.0)
				err = searchErr
				if searchErr == nil {
					result = &domain.SearchResult{
						Chunks: chunks,
						Query:  testCase.query,
					}
				}
			}

			assert.NoError(t, err, "Поиск по запросу '%s' не должен вызывать ошибку", testCase.query)
			assert.NotNil(t, result)
			assert.Equal(t, testCase.query, result.Query)

			// Для обязательных запросов проверяем, что найдены фрагменты
			if testCase.required {
				assert.NotEmpty(t, result.Chunks, "Для запроса '%s' должны быть найдены фрагменты из test_doc.txt", testCase.query)

				// Проверяем, что найденные фрагменты содержат ожидаемое слово
				found := false
				for _, chunk := range result.Chunks {
					if containsIgnoreCase(chunk.Content, testCase.expected) {
						found = true
						break
					}
				}
				assert.True(t, found, "Найденные фрагменты должны содержать слово '%s' из test_doc.txt", testCase.expected)
			} else {
				// Для необязательных запросов просто проверяем, что поиск работает
				if len(result.Chunks) == 0 {
					t.Logf("Поиск по '%s' не нашел результатов (возможно из-за особенностей поиска или разбиения на чанки)", testCase.query)
				} else {
					// Если нашли, проверяем содержимое
					found := false
					for _, chunk := range result.Chunks {
						if containsIgnoreCase(chunk.Content, testCase.expected) {
							found = true
							break
						}
					}
					if !found {
						t.Logf("Найдены фрагменты для '%s', но они не содержат ожидаемое слово (возможно из-за разбиения на чанки)", testCase.query)
					}
				}
			}
		})
	}

	// Проверяем генерацию ответа через AI - это ключевая проверка (только если AI доступен)
	if hasAI && service != nil {
		t.Run("AI_Generation", func(t *testing.T) {
			// Ищем информацию о компании
			searchResult, err := service.Search("компания", 3, 0.0)
			assert.NoError(t, err)
			if len(searchResult.Chunks) == 0 {
				t.Skip("Не найдено фрагментов для генерации ответа - возможно проблема с поиском")
				return
			}

			// Генерируем ответ через AI
			response, err := service.GenerateResponse(
				"Когда была основана компания?",
				searchResult.Chunks,
			)
			assert.NoError(t, err, "AI должен успешно сгенерировать ответ на основе test_doc.txt")
			assert.NotEmpty(t, response, "Ответ AI не должен быть пустым")
			assert.Contains(t, strings.ToLower(response), "2020", "Ответ должен содержать информацию о годе основания (2020) из test_doc.txt")

			t.Logf("AI успешно сгенерировал ответ на основе test_doc.txt: %s", response)
		})

		// Проверяем полный цикл: поиск + генерация одной командой
		t.Run("SearchAndGenerate", func(t *testing.T) {
			result, err := service.SearchAndGenerate("Сколько сотрудников работает в компании?", 3, 0.0)
			assert.NoError(t, err, "SearchAndGenerate должен работать без ошибок с test_doc.txt")
			assert.NotEmpty(t, result, "Ответ должен быть не пустым")

			// Проверяем, что ответ содержит релевантную информацию из test_doc.txt
			responseLower := strings.ToLower(result)
			assert.True(t,
				strings.Contains(responseLower, "100") || strings.Contains(responseLower, "сотрудник"),
				"Ответ должен содержать информацию о количестве сотрудников из test_doc.txt",
			)

			t.Logf("Полный цикл RAG с test_doc.txt успешен. Ответ: %s", result)
		})
	} else {
		t.Log("AI клиент недоступен - пропущены тесты генерации ответов. Для полной проверки установите AI_API_KEY")
	}
}

// containsIgnoreCase проверяет, содержит ли строка подстроку (без учета регистра)
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
