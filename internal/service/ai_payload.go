package service

const (
	geminiTemperature = 0.3
	geminiMaxTokens   = 2048
	ollamaTemperature = 0.3
	ollamaMaxTokens   = 2048
	kimiMaxTokens     = 2048
)

type promptPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []promptPart `json:"parts"`
}

type geminiGenerationConfig struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

type geminiChatRequest struct {
	SystemInstruction geminiContent          `json:"system_instruction"`
	Contents          []geminiContent        `json:"contents"`
	GenerationConfig  geminiGenerationConfig `json:"generationConfig"`
}

func newGeminiChatRequest(systemPrompt, userMessage string) geminiChatRequest {
	return geminiChatRequest{
		SystemInstruction: geminiContent{Parts: []promptPart{{Text: systemPrompt}}},
		Contents:          []geminiContent{{Parts: []promptPart{{Text: userMessage}}}},
		GenerationConfig: geminiGenerationConfig{
			Temperature:     geminiTemperature,
			MaxOutputTokens: geminiMaxTokens,
		},
	}
}

type geminiChatResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
	MaxTokens   int           `json:"max_tokens"`
	Stream      bool          `json:"stream,omitempty"`
}

func newOllamaChatRequest(model, systemPrompt, userMessage string, stream bool) ollamaChatRequest {
	return ollamaChatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		Temperature: ollamaTemperature,
		MaxTokens:   ollamaMaxTokens,
		Stream:      stream,
	}
}

type kimiChatRequest struct {
	Model               string        `json:"model"`
	Messages            []chatMessage `json:"messages"`
	MaxCompletionTokens int           `json:"max_completion_tokens"`
	Stream              bool          `json:"stream,omitempty"`
}

func newKimiChatRequest(model, systemPrompt, userMessage string, stream bool) kimiChatRequest {
	return kimiChatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMessage},
		},
		MaxCompletionTokens: kimiMaxTokens,
		Stream:              stream,
	}
}

type openAIChatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type openAIStreamResponse struct {
	Choices []struct {
		Delta chatMessage `json:"delta"`
	} `json:"choices"`
}
