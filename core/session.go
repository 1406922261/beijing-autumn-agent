package core

import (
	"github.com/openai/openai-go/v3"
)

// Session 保存多轮对话的消息历史（chat completions 格式）
type Session struct {
	Messages []openai.ChatCompletionMessageParamUnion
	Len      int
}

func NewSession() *Session {
	return &Session{
		Messages: []openai.ChatCompletionMessageParamUnion{},
		Len:      0,
	}
}

func (s *Session) GetMessages() []openai.ChatCompletionMessageParamUnion {
	return s.Messages
}

func (s *Session) GetLen() int {
	return s.Len
}

func (s *Session) AddMessage(item ...openai.ChatCompletionMessageParamUnion) {
	s.Messages = append(s.Messages, item...)
	s.Len++
}
