package data

import (
	"caichip/internal/conf"
	"context"
	"testing"
)

func TestChat(t *testing.T) {
	aiChat := NewOpenAIChat(&conf.Bootstrap{
		Openai: &conf.OpenAI{
			ApiKey:  "sk-DKPXsITvLkWVdtN_gTRPdTmv9ILx3VJ1FVy9AI8ur4R9P_3oM4INlil_Or8",
			BaseUrl: "http://10.86.3.248:3000/v1",
			Model:   "gpt-3.5-turbo",
		},
	})
	aiChat.Chat(context.Background(), "", "")
}
