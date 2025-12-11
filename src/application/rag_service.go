package application

import (
	"fmt"
	"rag-system/src/domain"
	"rag-system/src/infrastructure/ai"
)

// RAGService реализация сервиса RAG
type RAGService struct {
	repo domain.DocumentRepository
	ai   *ai.AIClient
}

// NewRAGService создает новый экземпляр RAG сервиса
func NewRAGService(repo domain.DocumentRepository, ai *ai.AIClient) *RAGService {
	return &RAGService{
		repo: repo,
		ai:   ai,
	}
}

// IndexDocument индексирует документ для поиска
func (s *RAGService) IndexDocument(doc domain.Document) error {
	return s.repo.SaveDocument(doc)
}

// Search ищет релевантную информацию по запросу
func (s *RAGService) Search(query string, limit int, threshold float64) (*domain.SearchResult, error) {
	chunks, err := s.repo.FindRelevantChunks(query, limit, threshold)
	if err != nil {
		return nil, fmt.Errorf("ошибка поиска: %w", err)
	}

	result := &domain.SearchResult{
		Chunks: chunks,
		Query:  query,
	}

	return result, nil
}

// GenerateResponse генерирует ответ на основе найденных фрагментов
func (s *RAGService) GenerateResponse(query string, chunks []domain.Chunk) (string, error) {
	response, err := s.ai.GenerateResponse(query, chunks)
	if err != nil {
		return "", fmt.Errorf("ошибка генерации ответа: %w", err)
	}

	return response, nil
}

// SearchAndGenerate объединяет поиск и генерацию ответа
func (s *RAGService) SearchAndGenerate(query string, limit int, threshold float64) (string, error) {
	searchResult, err := s.Search(query, limit, threshold)
	if err != nil {
		return "", fmt.Errorf("ошибка поиска: %w", err)
	}

	if len(searchResult.Chunks) == 0 {
		return "Не найдено релевантной информации для запроса.", nil
	}

	response, err := s.GenerateResponse(query, searchResult.Chunks)
	if err != nil {
		return "", fmt.Errorf("ошибка генерации ответа: %w", err)
	}

	return response, nil
}

// GetAllDocuments возвращает все документы
func (s *RAGService) GetAllDocuments() ([]domain.Document, error) {
	return s.repo.GetAllDocuments()
}
