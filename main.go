package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"rag-system/src/application"
	"rag-system/src/domain"
	"rag-system/src/infrastructure"
	"rag-system/src/infrastructure/ai"
	"time"
)

func main() {
	// Определяем флаги командной строки
	configPath := flag.String("config", "config/config.yaml", "Путь к файлу конфигурации")
	dbPath := flag.String("db", "./rag_system.db", "Путь к файлу базы данных")
	action := flag.String("action", "serve", "Действие: serve, index, search")
	docPath := flag.String("doc", "", "Путь к документу для индексации (для действия index)")
	query := flag.String("query", "", "Поисковый запрос (для действия search)")

	flag.Parse()

	// Загружаем AI клиент
	aiClient, err := ai.NewAIClient(*configPath)
	if err != nil {
		log.Fatalf("Ошибка инициализации AI клиента: %v", err)
	}

	// Создаем репозиторий
	repo, err := infrastructure.NewSQLiteDocumentRepository(*dbPath)
	if err != nil {
		log.Fatalf("Ошибка инициализации репозитория: %v", err)
	}
	defer repo.Close()

	// Создаем сервис
	service := application.NewRAGService(repo, aiClient)

	switch *action {
	case "index":
		if *docPath == "" {
			log.Fatal("Для действия 'index' требуется указать путь к документу (-doc)")
		}
		if err := handleIndex(service, *docPath); err != nil {
			log.Fatalf("Ошибка индексации документа: %v", err)
		}
	case "search":
		if *query == "" {
			log.Fatal("Для действия 'search' требуется указать поисковый запрос (-query)")
		}
		if err := handleSearch(service, *query); err != nil {
			log.Fatalf("Ошибка поиска: %v", err)
		}
	case "demo":
		if err := runDemo(service); err != nil {
			log.Fatalf("Ошибка демонстрации: %v", err)
		}
	case "serve":
		fallthrough
	default:
		fmt.Println("RAG система запущена. Используйте флаги для выполнения действий:")
		fmt.Println("  -action=index -doc=path/to/doc.txt     # Индексировать документ")
		fmt.Println("  -action=search -query='your query'    # Поиск по индексу")
		fmt.Println("  -action=demo                          # Запустить демо-сессию")
	}
}

// handleIndex индексирует документ
func handleIndex(service *application.RAGService, docPath string) error {
	content, err := os.ReadFile(docPath)
	if err != nil {
		return fmt.Errorf("ошибка чтения документа: %w", err)
	}

	doc := domain.Document{
		ID:      docPath, // В реальном приложении использовать UUID
		Title:   docPath,
		Content: string(content),
	}

	fmt.Printf("Индексируем документ: %s...\n", docPath)
	err = service.IndexDocument(doc)
	if err != nil {
		return fmt.Errorf("ошибка индексации: %w", err)
	}

	fmt.Println("Документ успешно проиндексирован!")
	return nil
}

// handleSearch выполняет поиск и генерацию ответа
func handleSearch(service *application.RAGService, query string) error {
	fmt.Printf("Выполняем поиск по запросу: '%s'\n", query)

	response, err := service.SearchAndGenerate(query, 5, 0.1)
	if err != nil {
		return fmt.Errorf("ошибка поиска и генерации: %w", err)
	}

	fmt.Printf("Ответ: %s\n", response)
	return nil
}

// runDemo запускает демо-сессию
func runDemo(service *application.RAGService) error {
	fmt.Println("=== Демонстрация RAG системы ===")

	// Проверим, есть ли уже документы в базе
	allDocs, err := service.GetAllDocuments()
	if err != nil {
		fmt.Printf("Предупреждение: не удалось получить существующие документы: %v\n", err)
	}

	// Если база пуста, индексируем тестовые документы
	if len(allDocs) == 0 {
		// Индексируем несколько тестовых документов
		docs := []domain.Document{
			{
				ID:      "doc1_" + fmt.Sprint(time.Now().Unix()), // Добавляем временную метку, чтобы избежать дубликатов
				Title:   "Информация о компании",
				Content: "Наша компания была основана в 2020 году. Мы специализируемся на разработке программного обеспечения и предоставлении IT-услуг. У нас работает более 100 сотрудников в 5 офисах по всему миру.",
			},
			{
				ID:      "doc2_" + fmt.Sprint(time.Now().Unix()), // Добавляем временную метку, чтобы избежать дубликатов
				Title:   "Продукты компании",
				Content: "Мы предлагаем широкий спектр решений: веб-приложения, мобильные приложения, системы анализа данных и искусственного интеллекта. Наши продукты используют более чем 500 компаний по всему миру.",
			},
			{
				ID:      "doc3_" + fmt.Sprint(time.Now().Unix()), // Добавляем временную метку, чтобы избежать дубликатов
				Title:   "Контактная информация",
				Content: "Главный офис находится в Москве. Адрес: улица Тверская, 1. Телефон: +7 (495) 123-45-67. Email: info@company.com. Режим работы: понедельник-пятница с 9:00 до 18:00.",
			},
		}

		fmt.Println("Индексируем тестовые документы...")
		for _, doc := range docs {
			err := service.IndexDocument(doc)
			if err != nil {
				return fmt.Errorf("ошибка индексации документа %s: %w", doc.Title, err)
			}
		}
	} else {
		fmt.Printf("База данных уже содержит %d документов\n", len(allDocs))
	}

	fmt.Println("Тестовые документы успешно проиндексированы!")

	// Выполняем несколько тестовых запросов
	queries := []string{
		"Когда была основана компания?",
		"Какие продукты предлагает компания?",
		"Где находится главный офис?",
	}

	for _, q := range queries {
		fmt.Printf("\nЗапрос: %s\n", q)

		// Сначала выполним поиск, чтобы показать, что система находит релевантные фрагменты
		searchResult, err := service.Search(q, 3, 0.01)
		if err != nil {
			fmt.Printf("Ошибка поиска: %v\n", err)
			continue
		}

		fmt.Printf("Найдено %d релевантных фрагментов\n", len(searchResult.Chunks))
		if len(searchResult.Chunks) > 0 {
			fmt.Println("Фрагменты:")
			for i, chunk := range searchResult.Chunks {
				fmt.Printf("  %d. [Similarity: %.2f] %s\n", i+1, chunk.Similarity,
					trimString(chunk.Content, 100)) // Показываем первые 100 символов
			}
		}

		// Попробуем сгенерировать ответ (может не получиться без действующего API ключа)
		response, err := service.SearchAndGenerate(q, 3, 0.01)
		if err != nil {
			fmt.Printf("Примечание: Не удалось сгенерировать ответ (возможно, проблема с API ключом): %v\n", err)
			fmt.Println("Но поиск работает корректно!")
		} else {
			fmt.Printf("Ответ: %s\n", response)
		}
	}

	return nil
}

// trimString обрезает строку до заданной длины
func trimString(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	return str[:maxLen] + "..."
}
