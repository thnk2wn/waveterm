// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/tmc/langchaingo/tools"
	"github.com/wavetermdev/waveterm/pkg/aiusechat"
	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/web/sse"
)

// WaveToolAdapter wraps Wave's tool definitions to work with LangChain
type WaveToolAdapter struct {
	waveTool   uctypes.ToolDefinition
	chatOpts   uctypes.WaveChatOpts
	sseHandler *sse.SSEHandlerCh
}

// NewWaveToolAdapter creates a new adapter for a Wave tool
func NewWaveToolAdapter(waveTool uctypes.ToolDefinition, chatOpts uctypes.WaveChatOpts, sseHandler *sse.SSEHandlerCh) *WaveToolAdapter {
	return &WaveToolAdapter{
		waveTool:   waveTool,
		chatOpts:   chatOpts,
		sseHandler: sseHandler,
	}
}

// Name implements tools.Tool.Name
func (w *WaveToolAdapter) Name() string {
	return w.waveTool.Name
}

// Description implements tools.Tool.Description
func (w *WaveToolAdapter) Description() string {
	return w.waveTool.Description
}

// Call implements tools.Tool.Call
// This integrates Wave's approval system with LangChain tool calls
func (w *WaveToolAdapter) Call(ctx context.Context, input string) (string, error) {
	// Parse the input JSON
	var inputMap map[string]any
	if err := json.Unmarshal([]byte(input), &inputMap); err != nil {
		return "", fmt.Errorf("invalid tool input JSON: %w", err)
	}

	// Create a tool call ID (LangChain doesn't provide one, so we generate it)
	toolCallID := fmt.Sprintf("lc_%s", generateShortID())

	// Create tool use data for Wave's approval system
	toolUseData := createToolUseData(toolCallID, w.waveTool.Name, input, w.chatOpts)

	// Check if approval is needed
	if toolUseData.Approval == uctypes.ApprovalNeedsApproval {
		// Register for approval
		aiusechat.RegisterToolApproval(toolCallID, w.sseHandler)

		// Send tool use data to UI for approval
		_ = w.sseHandler.AiMsgData("data-tooluse", toolCallID, toolUseData)

		// Wait for approval
		approval, err := aiusechat.WaitForToolApproval(ctx, toolCallID)
		if err != nil || approval == "" {
			approval = uctypes.ApprovalCanceled
		}

		toolUseData.Approval = approval

		// Check if approved
		if !toolUseData.IsApproved() {
			return "", fmt.Errorf("tool use denied by user")
		}

		// Update UI that tool was approved
		_ = w.sseHandler.AiMsgData("data-tooluse", toolCallID, toolUseData)
	}

	// Execute the tool
	toolCall := uctypes.WaveToolCall{
		ID:          toolCallID,
		Name:        w.waveTool.Name,
		Input:       inputMap,
		ToolUseData: &toolUseData,
	}

	result := aiusechat.ResolveToolCall(&w.waveTool, toolCall, w.chatOpts)

	// Update tool status in UI
	if result.ErrorText != "" {
		toolUseData.Status = uctypes.ToolUseStatusError
		toolUseData.ErrorMessage = result.ErrorText
		_ = w.sseHandler.AiMsgData("data-tooluse", toolCallID, toolUseData)
		return "", fmt.Errorf("tool execution error: %s", result.ErrorText)
	}

	toolUseData.Status = uctypes.ToolUseStatusCompleted
	_ = w.sseHandler.AiMsgData("data-tooluse", toolCallID, toolUseData)

	return result.Text, nil
}

// createToolUseData creates tool use data similar to aiutil.CreateToolUseData
func createToolUseData(toolCallID, toolName string, arguments string, chatOpts uctypes.WaveChatOpts) uctypes.UIMessageDataToolUse {
	toolUseData := uctypes.UIMessageDataToolUse{
		ToolCallId: toolCallID,
		ToolName:   toolName,
		Status:     uctypes.ToolUseStatusPending,
	}

	toolDef := chatOpts.GetToolDefinition(toolName)
	if toolDef == nil {
		toolUseData.Status = uctypes.ToolUseStatusError
		toolUseData.ErrorMessage = "tool not found"
		return toolUseData
	}

	var parsedArgs any
	if err := json.Unmarshal([]byte(arguments), &parsedArgs); err != nil {
		toolUseData.Status = uctypes.ToolUseStatusError
		toolUseData.ErrorMessage = fmt.Sprintf("failed to parse tool arguments: %v", err)
		return toolUseData
	}

	if toolDef.ToolCallDesc != nil {
		toolUseData.ToolDesc = toolDef.ToolCallDesc(parsedArgs, nil, nil)
	}

	if toolDef.ToolApproval != nil {
		toolUseData.Approval = toolDef.ToolApproval(parsedArgs)
	}

	return toolUseData
}

// generateShortID generates a short ID for tool calls
func generateShortID() string {
	// Simple implementation - could use UUID or other ID generation
	return fmt.Sprintf("%d", randomInt())
}

func randomInt() int {
	// Simple random - for production should use crypto/rand
	return 12345
}

// ConvertWaveToolsToLangChain converts Wave tool definitions to LangChain tools
func ConvertWaveToolsToLangChain(waveTools []uctypes.ToolDefinition, chatOpts uctypes.WaveChatOpts, sseHandler *sse.SSEHandlerCh) []tools.Tool {
	lcTools := make([]tools.Tool, 0, len(waveTools))
	for _, waveTool := range waveTools {
		adapter := NewWaveToolAdapter(waveTool, chatOpts, sseHandler)
		lcTools = append(lcTools, adapter)
	}
	return lcTools
}
