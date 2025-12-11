package infrastructure

import (
	"fmt"
	"log"
	"rag-system/src/domain"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteDocumentRepository реализация репозитория с использованием SQLite
type SQLiteDocumentRepository struct {
	db *sqlx.DB
}

// NewSQLiteDocumentRepository создает новый экземпляр репозитория
func NewSQLiteDocumentRepository(dbPath string) (*SQLiteDocumentRepository, error) {
	db, err := sqlx.Connect("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось подключиться к базе данных: %w", err)
	}

	repo := &SQLiteDocumentRepository{db: db}
	err = repo.initSchema()
	if err != nil {
		return nil, fmt.Errorf("не удалось инициализировать схему: %w", err)
	}

	return repo, nil
}

// initSchema инициализирует схему базы данных
func (r *SQLiteDocumentRepository) initSchema() error {
	tables := []string{
		`CREATE TABLE IF NOT EXISTS documents (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS chunks (
			id TEXT PRIMARY KEY,
			document_id TEXT NOT NULL,
			content TEXT NOT NULL,
			FOREIGN KEY(document_id) REFERENCES documents(id)
		)`,

		// Индекс для быстрого поиска по содержимому
		`CREATE INDEX IF NOT EXISTS idx_chunks_content ON chunks(content)`,
	}

	for _, tableSQL := range tables {
		_, err := r.db.Exec(tableSQL)
		if err != nil {
			log.Printf("Ошибка выполнения SQL: %s, ошибка: %v", tableSQL, err)
			return fmt.Errorf("ошибка при создании таблицы: %w", err)
		}
	}

	return nil
}

// SaveDocument сохраняет документ в базе данных
func (r *SQLiteDocumentRepository) SaveDocument(doc domain.Document) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("не удалось начать транзакцию: %w", err)
	}
	defer tx.Rollback()

	// Сохраняем документ
	stmt, err := tx.Prepare(`INSERT INTO documents (id, title, content) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("не удалось подготовить SQL для документа: %w", err)
	}
	defer stmt.Close()

	_, err = stmt.Exec(doc.ID, doc.Title, doc.Content)
	if err != nil {
		return fmt.Errorf("не удалось вставить документ: %w", err)
	}

	// Разбиваем документ на фрагменты (в реальном приложении использовать токенизацию)
	chunks := splitIntoChunks(doc.Content, 500) // Разбиваем на фрагменты по 500 символов

	for i, chunkText := range chunks {
		chunkID := fmt.Sprintf("%s_chunk_%d", doc.ID, i)
		chunkStmt, err := tx.Prepare(`INSERT INTO chunks (id, document_id, content) VALUES (?, ?, ?)`)
		if err != nil {
			return fmt.Errorf("не удалось подготовить SQL для фрагмента: %w", err)
		}
		defer chunkStmt.Close()

		_, err = chunkStmt.Exec(chunkID, doc.ID, chunkText)
		if err != nil {
			return fmt.Errorf("не удалось вставить фрагмент: %w", err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("не удалось зафиксировать транзакцию: %w", err)
	}

	return nil
}

// splitIntoChunks разбивает текст на фрагменты заданного размера
func splitIntoChunks(text string, chunkSize int) []string {
	var chunks []string

	for len(text) > 0 {
		if len(text) <= chunkSize {
			chunks = append(chunks, text)
			break
		}

		// Найдем наиболее подходящее место для разбиения (по предложению или абзацу)
		end := chunkSize
		for end > 0 && !isBreakPoint(rune(text[end])) {
			end--
		}

		if end == 0 {
			// Если не нашли точку разбиения, берем просто chunkSize
			end = chunkSize
		}

		chunks = append(chunks, text[:end])
		text = text[end:]
	}

	return chunks
}

// isBreakPoint проверяет, является ли символ подходящей точкой для разбиения
func isBreakPoint(r rune) bool {
	switch r {
	case '.', '!', '?', ';', ':', ',', ' ', '\n', '\t':
		return true
	default:
		return false
	}
}

// FindRelevantChunks находит релевантные фрагменты по запросу
func (r *SQLiteDocumentRepository) FindRelevantChunks(query string, limit int, threshold float64) ([]domain.Chunk, error) {
	var chunks []domain.Chunk

	// Разбиваем запрос на слова для более гибкого поиска
	queryWords := strings.Fields(query)

	var rows *sqlx.Rows
	var err error

	if len(queryWords) == 0 {
		// Если нет слов в запросе, возвращаем все фрагменты
		rows, err = r.db.Queryx("SELECT id, document_id, content FROM chunks LIMIT ?", limit)
	} else if len(queryWords) == 1 {
		// Если одно слово, используем простой LIKE
		rows, err = r.db.Queryx(
			"SELECT id, document_id, content FROM chunks WHERE content LIKE ? LIMIT ?",
			"%"+queryWords[0]+"%", limit,
		)
	} else {
		// Для нескольких слов создаем OR условие
		conditions := make([]string, len(queryWords))
		params := make([]interface{}, len(queryWords))

		for i, word := range queryWords {
			conditions[i] = "content LIKE ?"
			params[i] = "%" + word + "%"
		}

		conditionStr := strings.Join(conditions, " OR ")
		queryStr := fmt.Sprintf("SELECT id, document_id, content FROM chunks WHERE %s LIMIT ?", conditionStr)

		// Добавляем лимит к параметрам
		params = append(params, limit)

		rows, err = r.db.Queryx(queryStr, params...)
	}

	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var chunk domain.Chunk
		err := rows.Scan(&chunk.ID, &chunk.DocumentID, &chunk.Content)
		if err != nil {
			return nil, fmt.Errorf("ошибка сканирования строки: %w", err)
		}

		// Вычисляем примитивное сходство как количество совпадений слов
		chunk.Similarity = calculateSimpleSimilarity(query, chunk.Content)
		// Добавляем фрагмент, если сходство выше порога или если порог равен 0 (возвращаем все)
		if threshold <= 0 || chunk.Similarity >= threshold {
			chunks = append(chunks, chunk)
		}
	}

	return chunks, nil
}

// calculateSimpleSimilarity вычисляет простое сходство между запросом и содержимым
func calculateSimpleSimilarity(query, content string) float64 {
	queryWords := strings.Fields(strings.ToLower(query))
	contentLower := strings.ToLower(content)

	matches := 0
	for _, word := range queryWords {
		if strings.Contains(contentLower, word) {
			matches++
		}
	}

	if len(queryWords) == 0 {
		return 0
	}

	return float64(matches) / float64(len(queryWords))
}

// GetAllDocuments возвращает все документы
func (r *SQLiteDocumentRepository) GetAllDocuments() ([]domain.Document, error) {
	rows, err := r.db.Query("SELECT id, title, content, created_at FROM documents")
	if err != nil {
		return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer rows.Close()

	var docs []domain.Document
	for rows.Next() {
		var doc domain.Document
		var createdAtStr string
		err := rows.Scan(&doc.ID, &doc.Title, &doc.Content, &createdAtStr)
		if err != nil {
			return nil, fmt.Errorf("ошибка сканирования строки: %w", err)
		}

		docs = append(docs, doc)
	}

	return docs, nil
}

// DeleteDocument удаляет документ по ID
func (r *SQLiteDocumentRepository) DeleteDocument(id string) error {
	tx, err := r.db.Begin()
	if err != nil {
		return fmt.Errorf("не удалось начать транзакцию: %w", err)
	}
	defer tx.Rollback()

	// Удаляем связанные фрагменты
	_, err = tx.Exec("DELETE FROM chunks WHERE document_id=?", id)
	if err != nil {
		return fmt.Errorf("ошибка удаления фрагментов: %w", err)
	}

	// Удаляем сам документ
	_, err = tx.Exec("DELETE FROM documents WHERE id=?", id)
	if err != nil {
		return fmt.Errorf("ошибка удаления документа: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("не удалось зафиксировать транзакцию: %w", err)
	}

	return nil
}

// Close закрывает соединение с базой данных
func (r *SQLiteDocumentRepository) Close() error {
	return r.db.Close()
}
