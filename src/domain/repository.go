package domain

// DocumentRepository интерфейс для работы с документами
type DocumentRepository interface {
	// SaveDocument сохраняет документ в базе данных
	SaveDocument(doc Document) error

	// FindRelevantChunks находит релевантные фрагменты по запросу
	FindRelevantChunks(query string, limit int, threshold float64) ([]Chunk, error)

	// GetAllDocuments возвращает все документы
	GetAllDocuments() ([]Document, error)

	// DeleteDocument удаляет документ по ID
	DeleteDocument(id string) error
}
