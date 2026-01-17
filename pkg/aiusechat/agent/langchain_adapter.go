// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Package agent provides LangChain integration for Wave's AI agent mode.
// It adapts Wave's existing UseChatBackend and tool system to work with
// LangChain's agent executor framework.
package agent

import (
	"context"
	"fmt"

	"github.com/tmc/langchaingo/agents"
	"github.com/tmc/langchaingo/chains"
	"github.com/wavetermdev/waveterm/pkg/aiusechat"
	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/web/sse"
)

// WaveAgent wraps a LangChain agent executor for Wave Terminal
type WaveAgent struct {
	executor   *agents.Executor
	backend    aiusechat.UseChatBackend
	chatOpts   uctypes.WaveChatOpts
	sseHandler *sse.SSEHandlerCh
}

// CreateWaveAgent creates a new agent with Wave adapters
func CreateWaveAgent(backend aiusechat.UseChatBackend, tools []uctypes.ToolDefinition, chatOpts uctypes.WaveChatOpts, sseHandler *sse.SSEHandlerCh) (*WaveAgent, error) {
	// Create the LLM adapter
	llmAdapter := NewWaveLLMAdapter(backend, chatOpts, sseHandler)

	// Convert Wave tools to LangChain tools
	lcTools := ConvertWaveToolsToLangChain(tools, chatOpts, sseHandler)

	// Create a ReAct agent (Reasoning + Acting pattern)
	agentOptions := []agents.Option{
		agents.WithMaxIterations(10), // Default max iterations
	}

	// Create the agent
	agent := agents.NewOneShotAgent(
		llmAdapter,
		lcTools,
		agentOptions...,
	)

	// Create the executor
	executor := agents.NewExecutor(agent)

	return &WaveAgent{
		executor:   executor,
		backend:    backend,
		chatOpts:   chatOpts,
		sseHandler: sseHandler,
	}, nil
}

// Run executes the agent with the given task
func (w *WaveAgent) Run(ctx context.Context, task string) (string, error) {
	// Use chains.Run to execute the agent
	result, err := chains.Run(ctx, w.executor, task)
	if err != nil {
		return "", fmt.Errorf("agent execution failed: %w", err)
	}

	return result, nil
}

// SetMaxIterations sets the maximum number of iterations for the agent
func (w *WaveAgent) SetMaxIterations(max int) {
	// This would require recreating the agent with new options
	// For now, this is a placeholder
}
