package data

import (
	"context"
	"fmt"
	"strings"

	"caichip/internal/conf"

	openai "github.com/sashabaranov/go-openai"
)

// OpenAIChat 封装 Chat Completions（BOM parse_mode=llm 等场景）。
type OpenAIChat struct {
	client *openai.Client
	model  string
}

// NewOpenAIChat 从配置创建客户端；api_key 为空时返回 nil（调用方需拒绝 llm 模式）。
func NewOpenAIChat(bc *conf.Bootstrap) *OpenAIChat {
	if bc == nil {
		return nil
	}
	o := bc.GetOpenai()
	if o == nil {
		return nil
	}
	key := strings.TrimSpace(o.GetApiKey())
	if key == "" {
		return nil
	}
	cfg := openai.DefaultConfig(key)
	if u := strings.TrimSpace(o.GetBaseUrl()); u != "" {
		cfg.BaseURL = u
	}
	model := strings.TrimSpace(o.GetModel())
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAIChat{
		client: openai.NewClientWithConfig(cfg),
		model:  model,
	}
}

// Chat 单次 system + user 对话，返回 assistant 文本。
func (c *OpenAIChat) Chat(ctx context.Context, system, user string) (string, error) {
	if c == nil || c.client == nil {
		return "", fmt.Errorf("openai client not configured")
	}
	resp, err := c.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: c.model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: system},
			{Role: openai.ChatMessageRoleUser, Content: user},
		},
		Temperature: 0.1,
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("openai: empty choices")
	}
	return resp.Choices[0].Message.Content, nil
}
