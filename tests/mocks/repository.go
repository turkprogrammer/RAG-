package mocks

import (
	"rag-system/src/domain"
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

		chunkID := doc.ID + "_chunk_" + string(rune(len(chunks)))
		chunk := domain.Chunk{
			ID:         chunkID,
			DocumentID: doc.ID,
			Content:    content[i:end],
			Similarity: 0.5, // Для тестов устанавливаем произвольное значение
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

	// Возвращаем все фрагменты из всех документов для простоты тестирования
	var allChunks []domain.Chunk
	for _, chunks := range m.Chunks {
		for _, chunk := range chunks {
			allChunks = append(allChunks, chunk)
		}
	}

	// Ограничиваем результат в соответствии с лимитом
	if limit > 0 && len(allChunks) > limit {
		allChunks = allChunks[:limit]
	}

	return allChunks, nil
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
