package ai

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"rag-system/src/domain"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config структура конфигурации AI
type Config struct {
	AI struct {
		BaseURL     string  `yaml:"base_url"`
		APIKey      string  `yaml:"api_key"`
		Model       string  `yaml:"model"`
		TimeoutSecs int     `yaml:"timeout"` // Теперь это просто число секунд
		MaxTokens   int     `yaml:"max_tokens"`
		Temperature float64 `yaml:"temperature"`
	} `yaml:"ai"`
	Window struct {
		Width   int     `yaml:"width"`
		Height  int     `yaml:"height"`
		Opacity float64 `yaml:"opacity"`
	} `yaml:"window"`
	Logging struct {
		Level string `yaml:"level"`
	} `yaml:"logging"`
}

// AIClient клиент для взаимодействия с AI API
type AIClient struct {
	config     Config
	client     *http.Client
	cacheDir   string
	cacheMutex sync.RWMutex
	maxRetries int
	retryDelay time.Duration
	logger     *log.Logger
}

// RequestMetrics метрики запроса к AI API
type RequestMetrics struct {
	Duration  time.Duration
	Status    int
	Retries   int
	FromCache bool
	Error     error
}

// NewAIClient создает новый экземпляр AI клиента
func NewAIClient(configPath string) (*AIClient, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось загрузить конфигурацию: %w", err)
	}

	// Проверяем и приоритезируем переменные окружения
	// API ключ ОБЯЗАТЕЛЬНО должен быть установлен через переменную окружения
	apiKey := os.Getenv("AI_API_KEY")
	if apiKey == "" {
		// Проверяем, что в YAML не остался реальный ключ (должен быть только плейсхолдер)
		if config.AI.APIKey == "" || config.AI.APIKey == "YOUR_API_KEY_HERE" {
			return nil, fmt.Errorf("API ключ не установлен: необходимо установить переменную окружения AI_API_KEY. " +
				"Не храните секреты в config.yaml, используйте переменные окружения или secret management системы")
		}
		// Если в YAML обнаружен реальный ключ (не плейсхолдер), выдаем предупреждение
		// но все равно требуем env переменную для безопасности
		return nil, fmt.Errorf("API ключ должен быть установлен через переменную окружения AI_API_KEY, " +
			"а не через config.yaml. Обнаружено значение в config.yaml - это небезопасно. " +
			"Установите: export AI_API_KEY=\"ваш_ключ\"")
	}
	// Приоритет: env переменная всегда перезаписывает значение из YAML
	config.AI.APIKey = apiKey

	if model := os.Getenv("AI_MODEL"); model != "" {
		config.AI.Model = model
	}
	if baseURL := os.Getenv("AI_BASE_URL"); baseURL != "" {
		config.AI.BaseURL = baseURL
	}

	// Валидация обязательных параметров с явными ошибками
	if config.AI.BaseURL == "" {
		return nil, fmt.Errorf("конфигурация AI невалидна: поле 'base_url' обязательно и не может быть пустым. " +
			"Установите в config.yaml или через переменную окружения AI_BASE_URL")
	}
	if config.AI.Model == "" {
		return nil, fmt.Errorf("конфигурация AI невалидна: поле 'model' обязательно и не может быть пустым. " +
			"Установите в config.yaml или через переменную окружения AI_MODEL")
	}
	if config.AI.TimeoutSecs <= 0 {
		return nil, fmt.Errorf("конфигурация AI невалидна: поле 'timeout' должно быть положительным числом (секунды). "+
			"Текущее значение: %d", config.AI.TimeoutSecs)
	}
	if config.AI.MaxTokens <= 0 {
		return nil, fmt.Errorf("конфигурация AI невалидна: поле 'max_tokens' должно быть положительным числом. "+
			"Текущее значение: %d", config.AI.MaxTokens)
	}
	if config.AI.Temperature < 0 || config.AI.Temperature > 2 {
		return nil, fmt.Errorf("конфигурация AI невалидна: поле 'temperature' должно быть в диапазоне [0, 2]. "+
			"Текущее значение: %.2f", config.AI.Temperature)
	}

	// Создаем директорию для кэша
	cacheDir := filepath.Join(".", "cache", "ai")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("не удалось создать директорию для кэша: %w", err)
	}

	httpClient := &http.Client{
		Timeout: time.Duration(config.AI.TimeoutSecs) * time.Second,
	}

	logger := log.New(os.Stderr, "[AI] ", log.LstdFlags|log.Lshortfile)

	return &AIClient{
		config:     config,
		client:     httpClient,
		cacheDir:   cacheDir,
		maxRetries: 3,
		retryDelay: 2 * time.Second,
		logger:     logger,
	}, nil
}

// LoadConfig загружает конфигурацию из YAML файла
func LoadConfig(path string) (Config, error) {
	var config Config

	data, err := os.ReadFile(path)
	if err != nil {
		return config, fmt.Errorf("ошибка чтения файла конфигурации: %w", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("ошибка парсинга YAML: %w", err)
	}

	return config, nil
}

// sanitizeInput очищает и валидирует пользовательский ввод
func sanitizeInput(input string, maxLength int) string {
	// Удаляем управляющие символы и нормализуем пробелы
	cleaned := strings.TrimSpace(input)
	cleaned = strings.ReplaceAll(cleaned, "\x00", "") // Удаляем null байты

	// Ограничиваем длину
	if maxLength > 0 && len(cleaned) > maxLength {
		cleaned = cleaned[:maxLength]
	}

	return cleaned
}

// getCacheKey создает ключ кэша на основе запроса и контекста
func (c *AIClient) getCacheKey(query string, chunks []domain.Chunk) string {
	// Создаем уникальный ключ из запроса и содержимого чанков
	keyData := query
	for _, chunk := range chunks {
		keyData += chunk.ID + chunk.Content[:min(100, len(chunk.Content))]
	}

	hash := md5.Sum([]byte(keyData))
	return fmt.Sprintf("%x", hash)
}

// getCachedResponse получает ответ из кэша
func (c *AIClient) getCachedResponse(cacheKey string) (string, bool) {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()

	cacheFile := filepath.Join(c.cacheDir, cacheKey+".txt")
	data, err := os.ReadFile(cacheFile)
	if err != nil {
		return "", false
	}

	response := strings.TrimSpace(string(data))
	if response == "" {
		return "", false
	}

	c.logger.Printf("[CACHE] Использован кэш для запроса (ключ: %s)", cacheKey[:8])
	return response, true
}

// saveCachedResponse сохраняет ответ в кэш
func (c *AIClient) saveCachedResponse(cacheKey string, response string) error {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	cacheFile := filepath.Join(c.cacheDir, cacheKey+".txt")
	return os.WriteFile(cacheFile, []byte(response), 0644)
}

// logRequest логирует запрос с метриками
func (c *AIClient) logRequest(level, message string, metrics *RequestMetrics) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	var metricsStr string
	if metrics != nil {
		metricsStr = fmt.Sprintf(" [duration=%v, status=%d, retries=%d, cache=%v]",
			metrics.Duration, metrics.Status, metrics.Retries, metrics.FromCache)
		if metrics.Error != nil {
			metricsStr += fmt.Sprintf(" [error=%v]", metrics.Error)
		}
	}

	c.logger.Printf("[%s] %s: %s%s", timestamp, level, message, metricsStr)
}

// min возвращает минимум из двух чисел
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GenerateResponse генерирует ответ на основе контекста и запроса
func (c *AIClient) GenerateResponse(query string, contextChunks []domain.Chunk) (string, error) {
	startTime := time.Now()
	metrics := &RequestMetrics{}

	// Санитаризация входных данных
	query = sanitizeInput(query, 1000) // Максимум 1000 символов для запроса

	// Проверяем кэш
	cacheKey := c.getCacheKey(query, contextChunks)
	if cached, found := c.getCachedResponse(cacheKey); found {
		metrics.FromCache = true
		metrics.Duration = time.Since(startTime)
		c.logRequest("INFO", "Ответ получен из кэша", metrics)
		return cached, nil
	}

	// Создаем промпт с санитаризацией
	prompt := buildPrompt(query, contextChunks)

	// Ограничиваем размер промпта (защита от слишком больших запросов)
	maxPromptSize := 50000 // ~50KB символов
	if len(prompt) > maxPromptSize {
		c.logRequest("WARN", fmt.Sprintf("Промпт слишком большой (%d символов), обрезаем до %d", len(prompt), maxPromptSize), nil)
		prompt = prompt[:maxPromptSize] + "..."
	}

	payload := map[string]interface{}{
		"model":       c.config.AI.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  c.config.AI.MaxTokens,
		"temperature": c.config.AI.Temperature,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		metrics.Error = err
		metrics.Duration = time.Since(startTime)
		c.logRequest("ERROR", "Ошибка маршалинга JSON", metrics)
		return "", fmt.Errorf("ошибка маршалинга JSON: %w", err)
	}

	// Выполняем запрос с ретраями
	var response string
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		metrics.Retries = attempt

		if attempt > 0 {
			// Exponential backoff: 2s, 4s, 8s
			delay := c.retryDelay * time.Duration(1<<uint(attempt-1))
			c.logRequest("WARN", fmt.Sprintf("Повторная попытка %d/%d через %v", attempt, c.maxRetries, delay), nil)
			time.Sleep(delay)
		}

		// Создаем контекст с таймаутом для каждого запроса
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.config.AI.TimeoutSecs)*time.Second)

		req, err := http.NewRequestWithContext(ctx, "POST", c.config.AI.BaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
		if err != nil {
			cancel()
			lastErr = fmt.Errorf("ошибка создания запроса: %w", err)
			continue
		}

		req.Header.Set("Authorization", "Bearer "+c.config.AI.APIKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		cancel()

		if err != nil {
			lastErr = fmt.Errorf("ошибка выполнения запроса: %w", err)
			// Для ошибок сети/таймаута продолжаем ретраи
			if attempt < c.maxRetries {
				continue
			}
			break
		}

		metrics.Status = resp.StatusCode

		// Читаем тело ответа
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if readErr != nil {
			lastErr = fmt.Errorf("ошибка чтения ответа: %w", readErr)
			if attempt < c.maxRetries {
				continue
			}
			break
		}

		// Обработка различных HTTP статусов
		if resp.StatusCode == http.StatusOK {
			// Успешный ответ
			response, err = c.parseAIResponse(body)
			if err != nil {
				lastErr = err
				if attempt < c.maxRetries {
					continue
				}
				break
			}

			// Сохраняем в кэш
			if saveErr := c.saveCachedResponse(cacheKey, response); saveErr != nil {
				c.logRequest("WARN", fmt.Sprintf("Не удалось сохранить в кэш: %v", saveErr), nil)
			}

			metrics.Duration = time.Since(startTime)
			c.logRequest("INFO", "Успешный запрос к AI API", metrics)
			return response, nil

		} else if resp.StatusCode == http.StatusTooManyRequests { // 429
			c.logRequest("WARN", fmt.Sprintf("HTTP 429: Превышен лимит запросов (попытка %d/%d)", attempt+1, c.maxRetries+1), nil)

			// Пытаемся извлечь информацию о задержке из заголовков
			if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
				if delay, err := time.ParseDuration(retryAfter + "s"); err == nil {
					c.logRequest("INFO", fmt.Sprintf("Сервер запросил задержку: %v", delay), nil)
					time.Sleep(delay)
				}
			}

			if attempt < c.maxRetries {
				lastErr = fmt.Errorf("HTTP 429: превышен лимит запросов")
				continue
			}
			lastErr = fmt.Errorf("HTTP 429: превышен лимит запросов после %d попыток", c.maxRetries+1)
			break

		} else if resp.StatusCode >= 500 { // 5xx ошибки
			c.logRequest("WARN", fmt.Sprintf("HTTP %d: серверная ошибка (попытка %d/%d)", resp.StatusCode, attempt+1, c.maxRetries+1), nil)

			if attempt < c.maxRetries {
				lastErr = fmt.Errorf("HTTP %d: серверная ошибка", resp.StatusCode)
				continue
			}
			lastErr = fmt.Errorf("HTTP %d: серверная ошибка после %d попыток. Тело ответа: %s",
				resp.StatusCode, c.maxRetries+1, string(body[:min(200, len(body))]))
			break

		} else {
			// Другие ошибки (4xx кроме 429)
			lastErr = fmt.Errorf("HTTP %d: ошибка API. Тело ответа: %s",
				resp.StatusCode, string(body[:min(200, len(body))]))
			// Для 4xx ошибок не делаем ретраи
			break
		}
	}

	metrics.Error = lastErr
	metrics.Duration = time.Since(startTime)
	c.logRequest("ERROR", "Не удалось получить ответ от AI API", metrics)
	return "", lastErr
}

// parseAIResponse парсит ответ от AI API
func (c *AIClient) parseAIResponse(body []byte) (string, error) {
	// Проверяем валидность JSON перед парсингом
	var testJSON interface{}
	if err := json.Unmarshal(body, &testJSON); err != nil {
		return "", fmt.Errorf("невалидный JSON ответ: %w. Тело: %s", err, string(body[:min(200, len(body))]))
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("ошибка парсинга JSON ответа: %w", err)
	}

	// Проверяем наличие ошибки в ответе
	if response.Error.Message != "" {
		return "", fmt.Errorf("ошибка API: %s (тип: %s)", response.Error.Message, response.Error.Type)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("API вернул пустой ответ (нет choices)")
	}

	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return "", fmt.Errorf("API вернул пустой контент в ответе")
	}

	return content, nil
}

// BuildPrompt создает промпт на основе запроса и контекста с санитаризацией
func BuildPrompt(query string, chunks []domain.Chunk) string {
	// Санитаризация запроса
	query = sanitizeInput(query, 1000)

	// Собираем контекст с санитаризацией каждого чанка
	contextParts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		// Ограничиваем размер каждого чанка и санитируем
		content := sanitizeInput(chunk.Content, 5000) // Максимум 5000 символов на чанк
		if content != "" {
			contextParts = append(contextParts, content)
		}
	}

	context := strings.Join(contextParts, "\n\n")

	return fmt.Sprintf(
		"Ответь на вопрос, используя только информацию из следующего контекста.\n\nКонтекст:\n%s\n\nВопрос: %s\n\nОтвет:",
		context, query,
	)
}

// buildPrompt внутренняя функция для создания промпта
func buildPrompt(query string, chunks []domain.Chunk) string {
	return BuildPrompt(query, chunks)
}

// ClearCache очищает кэш AI ответов
func (c *AIClient) ClearCache() error {
	c.cacheMutex.Lock()
	defer c.cacheMutex.Unlock()

	files, err := filepath.Glob(filepath.Join(c.cacheDir, "*.txt"))
	if err != nil {
		return fmt.Errorf("ошибка чтения директории кэша: %w", err)
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil {
			c.logRequest("WARN", fmt.Sprintf("Не удалось удалить файл кэша %s: %v", file, err), nil)
		}
	}

	c.logRequest("INFO", fmt.Sprintf("Кэш очищен (%d файлов)", len(files)), nil)
	return nil
}

// GetCacheStats возвращает статистику кэша
func (c *AIClient) GetCacheStats() (int, error) {
	c.cacheMutex.RLock()
	defer c.cacheMutex.RUnlock()

	files, err := filepath.Glob(filepath.Join(c.cacheDir, "*.txt"))
	if err != nil {
		return 0, fmt.Errorf("ошибка чтения директории кэша: %w", err)
	}

	return len(files), nil
}
