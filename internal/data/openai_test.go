package data

import (
	"caichip/internal/conf"
	"context"
	"testing"
)

func TestChat(t *testing.T) {
	aiChat := NewOpenAIChat(&conf.Bootstrap{})
	aiChat.Chat(context.Background(), "", "")
}
