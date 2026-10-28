package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type chatRequest struct {
	Model           string    `json:"model"`
	Messages        []message `json:"messages"`
	Temperature     float64   `json:"temperature,omitempty"`
	MaxTokens       int       `json:"max_tokens,omitempty"`
	Stream          bool      `json:"stream"`
	ReasoningEffort string    `json:"reasoning_effort,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type imagePart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL struct {
		URL string `json:"url"`
	} `json:"image_url,omitempty"`
}

func (m *Manager) Call(agent *Agent, prompt string) (string, error) {
	if agent == nil || strings.TrimSpace(agent.BaseURL) == "" {
		return "", fmt.Errorf("未配置 base_url")
	}
	if agent.APIKey == "" && !isFreeGateway(agent.BaseURL) {
		return "", fmt.Errorf("该 AI 未配置 api_key")
	}
	messages := []message{{Role: "system", Content: agent.SystemPrompt}}
	if s := strings.TrimSpace(agent.Skill); s != "" {
		messages = append(messages, message{Role: "system", Content: "附加能力说明：\n" + s})
	}
	messages = append(messages, message{Role: "user", Content: prompt})
	return m.do(agent, messages, nil)
}

func (m *Manager) CallVision(agent *Agent, prompt string, images []string) (string, error) {
	if agent == nil || strings.TrimSpace(agent.BaseURL) == "" {
		return "", fmt.Errorf("未配置 base_url")
	}
	if agent.APIKey == "" && !isFreeGateway(agent.BaseURL) {
		return "", fmt.Errorf("该 AI 未配置 api_key")
	}
	parts := []imagePart{{Type: "text", Text: prompt}}
	for _, img := range images {
		p := imagePart{Type: "image_url"}
		p.ImageURL.URL = img
		parts = append(parts, p)
	}
	messages := []message{
		{Role: "system", Content: agent.SystemPrompt},
		{Role: "user", Content: parts},
	}
	return m.do(agent, messages, images)
}

func (m *Manager) do(agent *Agent, messages []message, _ []string) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(agent.BaseURL), "/")
	url := base
	if !strings.HasSuffix(url, "/chat/completions") {
		url += "/chat/completions"
	}
	body := chatRequest{
		Model: agent.Model, Messages: messages, Temperature: agent.Temperature,
		MaxTokens: agent.MaxTokens, Stream: false,
	}
	if agent.ReasoningEffort != "" && agent.ReasoningEffort != "off" {
		body.ReasoningEffort = agent.ReasoningEffort
	}
	payload, _ := json.Marshal(body)
	client := &http.Client{Timeout: 150 * time.Second}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")
		if agent.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+agent.APIKey)
		}
		res, err := client.Do(req)
		if err != nil {
			lastErr = err
		} else {
			raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
			res.Body.Close()
			if res.StatusCode >= 200 && res.StatusCode < 300 {
				var parsed struct {
					Choices []struct {
						Message struct {
							Content          string `json:"content"`
							ReasoningContent string `json:"reasoning_content"`
						} `json:"message"`
						FinishReason string `json:"finish_reason"`
					} `json:"choices"`
					Error struct {
						Message string `json:"message"`
					} `json:"error"`
				}
				if err := json.Unmarshal(raw, &parsed); err != nil {
					return "", fmt.Errorf("AI 返回非 JSON")
				}
				if len(parsed.Choices) == 0 {
					if parsed.Error.Message != "" {
						return "", fmt.Errorf("AI 错误：%s", parsed.Error.Message)
					}
					return "", fmt.Errorf("AI 返回空 choices")
				}
				if parsed.Choices[0].Message.Content == "" {
					return "", fmt.Errorf("AI 返回空内容（finish_reason=%s）", parsed.Choices[0].FinishReason)
				}
				return parsed.Choices[0].Message.Content, nil
			}
			lastErr = fmt.Errorf("AI HTTP %d: %s", res.StatusCode, string(raw[:min(len(raw), 300)]))
			if res.StatusCode != 429 && res.StatusCode < 500 {
				return "", lastErr
			}
		}
		if attempt < 3 {
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}
	}
	return "", lastErr
}

func isFreeGateway(base string) bool {
	return strings.Contains(base, "opencode.ai") || strings.Contains(base, "zen/v1")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
