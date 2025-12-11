package domain

import "time"

// Document представляет документ, который будет индексироваться в системе
type Document struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

// Chunk представляет фрагмент документа для поиска
type Chunk struct {
	ID         string  `json:"id"`
	DocumentID string  `json:"document_id"`
	Content    string  `json:"content"`
	Similarity float64 `json:"similarity"` // Для релевантности
}

// SearchRequest структура запроса на поиск
type SearchRequest struct {
	Query     string  `json:"query"`
	Limit     int     `json:"limit"`
	Threshold float64 `json:"threshold"`
}

// SearchResult результаты поиска
type SearchResult struct {
	Chunks []Chunk `json:"chunks"`
	Query  string  `json:"query"`
}
