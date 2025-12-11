package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"rag-system/src/domain"
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
	config Config
	client *http.Client
}

// NewAIClient создает новый экземпляр AI клиента
func NewAIClient(configPath string) (*AIClient, error) {
	config, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("не удалось загрузить конфигурацию: %w", err)
	}

	// Проверяем и приоритезируем переменные окружения
	if apiKey := os.Getenv("AI_API_KEY"); apiKey != "" {
		config.AI.APIKey = apiKey
	}
	if model := os.Getenv("AI_MODEL"); model != "" {
		config.AI.Model = model
	}
	if baseURL := os.Getenv("AI_BASE_URL"); baseURL != "" {
		config.AI.BaseURL = baseURL
	}

	httpClient := &http.Client{
		Timeout: time.Duration(config.AI.TimeoutSecs) * time.Second,
	}

	return &AIClient{
		config: config,
		client: httpClient,
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

// GenerateResponse генерирует ответ на основе контекста и запроса
func (c *AIClient) GenerateResponse(query string, contextChunks []domain.Chunk) (string, error) {
	prompt := buildPrompt(query, contextChunks)

	payload := map[string]interface{}{
		"model":       c.config.AI.Model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"max_tokens":  c.config.AI.MaxTokens,
		"temperature": c.config.AI.Temperature,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("ошибка маршалинга JSON: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), "POST", c.config.AI.BaseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.config.AI.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ошибка API: статус %d, тело: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("ошибка чтения ответа: %w", err)
	}

	err = json.Unmarshal(body, &response)
	if err != nil {
		return "", fmt.Errorf("ошибка парсинга JSON ответа: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("API вернул пустой ответ")
	}

	return response.Choices[0].Message.Content, nil
}

// BuildPrompt создает промпт на основе запроса и контекста
func BuildPrompt(query string, chunks []domain.Chunk) string {
	context := ""
	for _, chunk := range chunks {
		context += chunk.Content + "\n\n"
	}

	return fmt.Sprintf(
		"Ответь на вопрос, используя только информацию из следующего контекста.\n\nКонтекст:\n%s\n\nВопрос: %s\n\nОтвет:",
		context, query,
	)
}

// buildPrompt внутренняя функция для создания промпта
func buildPrompt(query string, chunks []domain.Chunk) string {
	return BuildPrompt(query, chunks)
}
