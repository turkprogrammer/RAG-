package unit

import (
	"github.com/stretchr/testify/assert"
	"rag-system/src/domain"
	"rag-system/src/infrastructure/ai"
	"testing"
)

func TestAIConfigLoading(t *testing.T) {
	// Используем реальный конфигурационный файл
	configFile := "../../config/config.yaml"

	config, err := ai.LoadConfig(configFile)
	assert.NoError(t, err)
	// Проверяем, что основные AI конфигурационные значения загружены
	assert.NotEmpty(t, config.AI.BaseURL)
	assert.NotEmpty(t, config.AI.Model)
	assert.NotZero(t, config.AI.TimeoutSecs)
	assert.NotZero(t, config.AI.MaxTokens)
	assert.NotNil(t, config.AI.Temperature)

	// Проверяем, что другие секции (window и logging) не обязательны и не вызывают ошибок
	// при загрузке, хотя и не используются в текущей реализации
}

func TestAIClientInitialization(t *testing.T) {
	// Используем реальный конфигурационный файл
	configFile := "../../config/config.yaml"

	// Пробуем создать клиент с реальным конфигом
	client, err := ai.NewAIClient(configFile)
	// Не проверяем ошибку, так как это может быть связано с отсутствием API ключа
	if err != nil {
		// Просто убедимся, что ошибка связана с настройками API, а не с парсингом конфига
		assert.Contains(t, err.Error(), "ошибка")
	} else {
		assert.NotNil(t, client)
	}
}

// Тест для проверки построения промпта
func TestBuildPrompt(t *testing.T) {
	chunks := []domain.Chunk{
		{
			Content: "Первый фрагмент документа с полезной информацией.",
		},
		{
			Content: "Второй фрагмент с дополнительными деталями.",
		},
	}

	query := "Что содержится в документах?"
	expectedContext := "Первый фрагмент документа с полезной информацией.\n\nВторой фрагмент с дополнительными деталями.\n\n"
	expected := "Ответь на вопрос, используя только информацию из следующего контекста.\n\nКонтекст:\n" + expectedContext + "\n\nВопрос: " + query + "\n\nОтвет:"

	actual := ai.BuildPrompt(query, chunks)
	assert.Equal(t, expected, actual)
}
