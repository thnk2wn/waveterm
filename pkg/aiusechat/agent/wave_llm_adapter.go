// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"

	"github.com/tmc/langchaingo/llms"
	"github.com/wavetermdev/waveterm/pkg/aiusechat"
	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/web/sse"
)

// WaveLLMAdapter implements langchaingo's llms.Model interface using Wave's UseChatBackend
type WaveLLMAdapter struct {
	backend    aiusechat.UseChatBackend
	chatOpts   uctypes.WaveChatOpts
	sseHandler *sse.SSEHandlerCh
}

// NewWaveLLMAdapter creates a new adapter that connects Wave's backend to LangChain
func NewWaveLLMAdapter(backend aiusechat.UseChatBackend, chatOpts uctypes.WaveChatOpts, sseHandler *sse.SSEHandlerCh) *WaveLLMAdapter {
	return &WaveLLMAdapter{
		backend:    backend,
		chatOpts:   chatOpts,
		sseHandler: sseHandler,
	}
}

// GenerateContent implements llms.Model.GenerateContent
// This method is called by LangChain's agent executor to get completions from the LLM
func (w *WaveLLMAdapter) GenerateContent(ctx context.Context, messages []llms.MessageContent, options ...llms.CallOption) (*llms.ContentResponse, error) {
	// Convert LangChain messages to Wave format
	// For now, we'll use the existing chat history and just append the new message
	// LangChain manages its own message history in the agent executor

	// Run one step of the Wave chat backend
	stopReason, nativeMessages, _, err := w.backend.RunChatStep(ctx, w.sseHandler, w.chatOpts, nil)
	if err != nil {
		return nil, fmt.Errorf("wave backend error: %w", err)
	}

	// Convert the response back to LangChain format
	response := &llms.ContentResponse{
		Choices: []llms.ContentChoice{
			{
				Content: extractTextFromStopReason(stopReason, nativeMessages),
			},
		},
	}

	return response, nil
}

// Call implements llms.Model.Call (deprecated method, calls GenerateContent)
func (w *WaveLLMAdapter) Call(ctx context.Context, prompt string, options ...llms.CallOption) (string, error) {
	messages := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextPart(prompt),
			},
		},
	}

	response, err := w.GenerateContent(ctx, messages, options...)
	if err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no response from model")
	}

	return response.Choices[0].Content, nil
}

// extractTextFromStopReason extracts text content from Wave's response format
func extractTextFromStopReason(stopReason *uctypes.WaveStopReason, messages []uctypes.GenAIMessage) string {
	// TODO: Properly extract text from native messages
	// For now, return a placeholder that indicates we need to handle tool calls
	if stopReason.Kind == uctypes.StopKindToolUse {
		return fmt.Sprintf("[Tool calls pending: %d tools]", len(stopReason.ToolCalls))
	}

	// Extract text from the last assistant message
	if len(messages) > 0 {
		// This is a simplified extraction - we'll need to properly handle different message types
		return "[Assistant response]"
	}

	return ""
}
