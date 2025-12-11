package mocks

import (
	"fmt"
	"rag-system/src/domain"
	"strings"
)

// MockDocumentRepository имитация репозитория для тестирования
type MockDocumentRepository struct {
	Documents            map[string]domain.Document
	Chunks               map[string][]domain.Chunk
	SaveDocumentFn       func(doc domain.Document) error
	FindRelevantChunksFn func(query string, limit int, threshold float64) ([]domain.Chunk, error)
	GetAllDocumentsFn    func() ([]domain.Document, error)
	DeleteDocumentFn     func(id string) error
}

func NewMockDocumentRepository() *MockDocumentRepository {
	return &MockDocumentRepository{
		Documents: make(map[string]domain.Document),
		Chunks:    make(map[string][]domain.Chunk),
	}
}

func (m *MockDocumentRepository) SaveDocument(doc domain.Document) error {
	if m.SaveDocumentFn != nil {
		return m.SaveDocumentFn(doc)
	}

	// Сохраняем документ
	m.Documents[doc.ID] = doc

	// Создаем фрагменты из содержимого документа
	var chunks []domain.Chunk
	content := doc.Content
	chunkSize := 100
	for i := 0; i < len(content); i += chunkSize {
		end := i + chunkSize
		if end > len(content) {
			end = len(content)
		}

		chunkID := fmt.Sprintf("%s_chunk_%d", doc.ID, len(chunks))
		chunk := domain.Chunk{
			ID:         chunkID,
			DocumentID: doc.ID,
			Content:    content[i:end],
			Similarity: 0.5, // Будет пересчитано при поиске
		}
		chunks = append(chunks, chunk)
	}

	m.Chunks[doc.ID] = chunks
	return nil
}

func (m *MockDocumentRepository) FindRelevantChunks(query string, limit int, threshold float64) ([]domain.Chunk, error) {
	if m.FindRelevantChunksFn != nil {
		return m.FindRelevantChunksFn(query, limit, threshold)
	}

	// Имитируем реальный поиск: фильтруем по содержимому и вычисляем similarity
	query = strings.ToLower(strings.TrimSpace(query))
	queryWords := strings.Fields(query)

	var matchingChunks []domain.Chunk

	// Если запрос пустой, возвращаем все фрагменты
	if query == "" {
		for _, chunks := range m.Chunks {
			for _, chunk := range chunks {
				chunk.Similarity = 0.5
				matchingChunks = append(matchingChunks, chunk)
			}
		}
	} else {
		// Ищем фрагменты, содержащие слова запроса
		for _, chunks := range m.Chunks {
			for _, chunk := range chunks {
				contentLower := strings.ToLower(chunk.Content)

				// Вычисляем similarity как долю найденных слов
				matches := 0
				for _, word := range queryWords {
					if strings.Contains(contentLower, word) {
						matches++
					}
				}

				if len(queryWords) > 0 {
					chunk.Similarity = float64(matches) / float64(len(queryWords))
				} else {
					chunk.Similarity = 0.0
				}

				// Добавляем только если similarity >= threshold или threshold <= 0
				if threshold <= 0 || chunk.Similarity >= threshold {
					matchingChunks = append(matchingChunks, chunk)
				}
			}
		}

		// Сортируем по similarity (лучшие результаты первыми)
		// Простая сортировка пузырьком для небольшого количества данных
		for i := 0; i < len(matchingChunks)-1; i++ {
			for j := i + 1; j < len(matchingChunks); j++ {
				if matchingChunks[i].Similarity < matchingChunks[j].Similarity {
					matchingChunks[i], matchingChunks[j] = matchingChunks[j], matchingChunks[i]
				}
			}
		}
	}

	// Ограничиваем результат в соответствии с лимитом
	if limit > 0 && len(matchingChunks) > limit {
		matchingChunks = matchingChunks[:limit]
	}

	return matchingChunks, nil
}

func (m *MockDocumentRepository) GetAllDocuments() ([]domain.Document, error) {
	if m.GetAllDocumentsFn != nil {
		return m.GetAllDocumentsFn()
	}

	var docs []domain.Document
	for _, doc := range m.Documents {
		docs = append(docs, doc)
	}

	return docs, nil
}

func (m *MockDocumentRepository) DeleteDocument(id string) error {
	if m.DeleteDocumentFn != nil {
		return m.DeleteDocumentFn(id)
	}

	delete(m.Documents, id)
	delete(m.Chunks, id)
	return nil
}
