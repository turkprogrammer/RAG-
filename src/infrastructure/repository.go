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
	db          *sqlx.DB
	fts5Enabled bool // Флаг поддержки FTS5
}

// NewSQLiteDocumentRepository создает новый экземпляр репозитория
func NewSQLiteDocumentRepository(dbPath string) (*SQLiteDocumentRepository, error) {
	db, err := sqlx.Connect("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось подключиться к базе данных: %w", err)
	}

	repo := &SQLiteDocumentRepository{db: db, fts5Enabled: false}

	// Проверяем поддержку FTS5
	repo.fts5Enabled = repo.checkFTS5Support()

	err = repo.initSchema()
	if err != nil {
		return nil, fmt.Errorf("не удалось инициализировать схему: %w", err)
	}

	return repo, nil
}

// checkFTS5Support проверяет, поддерживает ли SQLite FTS5
func (r *SQLiteDocumentRepository) checkFTS5Support() bool {
	var result string
	err := r.db.Get(&result, "SELECT 'fts5' WHERE 'fts5' IN (SELECT name FROM pragma_module_list())")
	if err != nil {
		log.Printf("FTS5 не поддерживается в данной версии SQLite, используется fallback на LIKE поиск")
		return false
	}
	return result == "fts5"
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

		// Индекс для быстрого поиска по содержимому (fallback если FTS5 недоступен)
		`CREATE INDEX IF NOT EXISTS idx_chunks_content ON chunks(content)`,
	}

	// Добавляем FTS5 таблицу и триггеры только если FTS5 поддерживается
	if r.fts5Enabled {
		fts5Tables := []string{
			// FTS5 виртуальная таблица для полнотекстового поиска
			`CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
				content,
				content='chunks',
				content_rowid='rowid'
			)`,

			// Триггеры для автоматической синхронизации данных между chunks и chunks_fts
			`CREATE TRIGGER IF NOT EXISTS chunks_fts_insert AFTER INSERT ON chunks BEGIN
				INSERT INTO chunks_fts(rowid, content) VALUES (new.rowid, new.content);
			END`,

			`CREATE TRIGGER IF NOT EXISTS chunks_fts_update AFTER UPDATE OF content ON chunks BEGIN
				UPDATE chunks_fts SET content = new.content WHERE rowid = new.rowid;
			END`,

			`CREATE TRIGGER IF NOT EXISTS chunks_fts_delete AFTER DELETE ON chunks BEGIN
				DELETE FROM chunks_fts WHERE rowid = old.rowid;
			END`,
		}
		tables = append(tables, fts5Tables...)
	}

	for _, tableSQL := range tables {
		_, err := r.db.Exec(tableSQL)
		if err != nil {
			log.Printf("Ошибка выполнения SQL: %s, ошибка: %v", tableSQL, err)
			return fmt.Errorf("ошибка при создании таблицы: %w", err)
		}
	}

	// Миграция существующих данных в FTS5 индекс (только если FTS5 поддерживается)
	if r.fts5Enabled {
		err := r.rebuildFTSIndex()
		if err != nil {
			log.Printf("Предупреждение: не удалось переиндексировать FTS5: %v", err)
			// Не возвращаем ошибку, так как это может быть первичная инициализация
		}
	}

	return nil
}

// rebuildFTSIndex переиндексирует существующие данные в FTS5 таблицу
func (r *SQLiteDocumentRepository) rebuildFTSIndex() error {
	// Проверяем, есть ли уже данные в FTS5
	var count int
	err := r.db.Get(&count, "SELECT COUNT(*) FROM chunks_fts")
	if err == nil && count > 0 {
		// Проверяем, все ли данные синхронизированы
		var chunksCount int
		err = r.db.Get(&chunksCount, "SELECT COUNT(*) FROM chunks")
		if err == nil && count == chunksCount {
			// Индекс уже заполнен и синхронизирован
			return nil
		}
	}

	// Заполняем FTS5 индекс из существующих данных
	// Используем INSERT OR IGNORE для избежания дубликатов
	_, err = r.db.Exec(`
		INSERT INTO chunks_fts(rowid, content) 
		SELECT rowid, content FROM chunks
		WHERE rowid NOT IN (SELECT rowid FROM chunks_fts)
	`)
	if err != nil {
		return fmt.Errorf("ошибка переиндексации FTS5: %w", err)
	}

	log.Printf("FTS5 индекс успешно переиндексирован")
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

// formatFTS5Query форматирует пользовательский запрос для FTS5
// FTS5 поддерживает операторы: AND, OR, NOT, фразы в кавычках
func formatFTS5Query(query string) string {
	words := strings.Fields(strings.TrimSpace(query))
	if len(words) == 0 {
		return ""
	}

	// Экранируем специальные символы FTS5: ", ', \
	escapedWords := make([]string, 0, len(words))
	for _, word := range words {
		// Убираем специальные символы FTS5 для безопасности
		cleaned := strings.ReplaceAll(word, "\"", "")
		cleaned = strings.ReplaceAll(cleaned, "'", "")
		cleaned = strings.ReplaceAll(cleaned, "\\", "")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned != "" {
			escapedWords = append(escapedWords, cleaned)
		}
	}

	if len(escapedWords) == 0 {
		return ""
	}

	// Объединяем через AND (все слова должны быть найдены)
	// Можно использовать OR для более мягкого поиска, но AND дает более релевантные результаты
	return strings.Join(escapedWords, " AND ")
}

// FindRelevantChunks находит релевантные фрагменты по запросу используя FTS5 (если доступен) или LIKE (fallback)
func (r *SQLiteDocumentRepository) FindRelevantChunks(query string, limit int, threshold float64) ([]domain.Chunk, error) {
	// Используем FTS5 если доступен, иначе fallback на старый метод
	if r.fts5Enabled {
		return r.findRelevantChunksFTS5(query, limit, threshold)
	}
	return r.findRelevantChunksLike(query, limit, threshold)
}

// findRelevantChunksFTS5 находит релевантные фрагменты используя FTS5
func (r *SQLiteDocumentRepository) findRelevantChunksFTS5(query string, limit int, threshold float64) ([]domain.Chunk, error) {
	var chunks []domain.Chunk

	// Обработка пустого запроса
	if strings.TrimSpace(query) == "" {
		rows, err := r.db.Queryx(`
			SELECT id, document_id, content 
			FROM chunks 
			LIMIT ?`, limit)
		if err != nil {
			return nil, fmt.Errorf("ошибка выполнения запроса: %w", err)
		}
		defer rows.Close()

		for rows.Next() {
			var chunk domain.Chunk
			if err := rows.Scan(&chunk.ID, &chunk.DocumentID, &chunk.Content); err != nil {
				return nil, fmt.Errorf("ошибка сканирования: %w", err)
			}
			chunk.Similarity = 0.5 // Значение по умолчанию для пустого запроса
			if threshold <= 0 || chunk.Similarity >= threshold {
				chunks = append(chunks, chunk)
			}
		}
		return chunks, nil
	}

	// Форматируем запрос для FTS5
	ftsQuery := formatFTS5Query(query)
	if ftsQuery == "" {
		return chunks, nil
	}

	// FTS5 запрос с ранжированием через bm25()
	// bm25() возвращает отрицательные значения: чем меньше (ближе к 0), тем лучше совпадение
	querySQL := `
		SELECT 
			c.id,
			c.document_id,
			c.content,
			bm25(chunks_fts) AS rank_score
		FROM chunks c
		JOIN chunks_fts ON c.rowid = chunks_fts.rowid
		WHERE chunks_fts MATCH ?
		ORDER BY rank_score
		LIMIT ?`

	rows, err := r.db.Queryx(querySQL, ftsQuery, limit)
	if err != nil {
		// Если FTS5 таблица не существует или произошла ошибка, возвращаем ошибку
		return nil, fmt.Errorf("ошибка выполнения FTS5 запроса: %w", err)
	}
	defer rows.Close()

	// Собираем результаты с рангами для нормализации
	type chunkWithRank struct {
		chunk     domain.Chunk
		rankScore float64
	}
	var tempResults []chunkWithRank

	for rows.Next() {
		var cwr chunkWithRank
		if err := rows.Scan(&cwr.chunk.ID, &cwr.chunk.DocumentID, &cwr.chunk.Content, &cwr.rankScore); err != nil {
			return nil, fmt.Errorf("ошибка сканирования строки: %w", err)
		}
		tempResults = append(tempResults, cwr)
	}

	// Нормализуем ранги в similarity (0-1, где 1 = лучшее совпадение)
	// bm25() возвращает отрицательные значения: лучший результат имеет наименьшее (самое отрицательное) значение
	if len(tempResults) > 0 {
		// Находим минимальный и максимальный rank для нормализации
		minRank := tempResults[0].rankScore // Первый элемент уже отсортирован по rank_score (ASC)
		maxRank := tempResults[len(tempResults)-1].rankScore

		for _, result := range tempResults {
			// Инвертируем и нормализуем: лучший результат (min rank, самое отрицательное) = 1.0
			if maxRank == minRank {
				result.chunk.Similarity = 1.0
			} else {
				// Нормализация: (maxRank - currentRank) / (maxRank - minRank)
				// Поскольку rank отрицательный, это даст значение от 0 до 1
				result.chunk.Similarity = (maxRank - result.rankScore) / (maxRank - minRank)
			}

			// Применяем threshold фильтр
			if threshold <= 0 || result.chunk.Similarity >= threshold {
				chunks = append(chunks, result.chunk)
			}
		}
	}

	return chunks, nil
}

// findRelevantChunksLike находит релевантные фрагменты используя LIKE (fallback метод)
func (r *SQLiteDocumentRepository) findRelevantChunksLike(query string, limit int, threshold float64) ([]domain.Chunk, error) {
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

// calculateSimpleSimilarity вычисляет простое сходство между запросом и содержимым (для fallback метода)
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
