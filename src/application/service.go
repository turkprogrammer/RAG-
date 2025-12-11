package application

import "rag-system/src/domain"

// DocumentService интерфейс сервиса для управления документами
type DocumentService interface {
	// IndexDocument индексирует документ для поиска
	IndexDocument(doc domain.Document) error

	// Search ищет релевантную информацию по запросу
	Search(query string, limit int, threshold float64) (*domain.SearchResult, error)

	// GenerateResponse генерирует ответ на основе найденных фрагментов
	GenerateResponse(query string, chunks []domain.Chunk) (string, error)

	// GetAllDocuments возвращает все документы
	GetAllDocuments() ([]domain.Document, error)
}
