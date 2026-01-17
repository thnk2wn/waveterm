// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

package aiusechat

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/aiusechat/uctypes"
	"github.com/wavetermdev/waveterm/pkg/blockcontroller"
	"github.com/wavetermdev/waveterm/pkg/util/utilfn"
	"github.com/wavetermdev/waveterm/pkg/waveobj"
	"github.com/wavetermdev/waveterm/pkg/wcore"
	"github.com/wavetermdev/waveterm/pkg/wshrpc"
	"github.com/wavetermdev/waveterm/pkg/wstore"
)

const DefaultCommandTimeout = 300 // 5 minutes

type runTerminalCommandParams struct {
	WidgetId       string `json:"widget_id"`
	Command        string `json:"command"`
	TimeoutSeconds *int   `json:"timeout_seconds"`
}

func parseRunTerminalCommandInput(input any) (*runTerminalCommandParams, error) {
	result := &runTerminalCommandParams{}

	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	if err := utilfn.ReUnmarshal(result, input); err != nil {
		return nil, fmt.Errorf("invalid input format: %w", err)
	}

	if result.WidgetId == "" {
		return nil, fmt.Errorf("missing widget_id parameter")
	}

	if result.Command == "" {
		return nil, fmt.Errorf("missing command parameter")
	}

	if result.TimeoutSeconds == nil {
		timeout := DefaultCommandTimeout
		result.TimeoutSeconds = &timeout
	}

	return result, nil
}

// isDangerousCommand checks if a command matches dangerous patterns
func isDangerousCommand(command string) (bool, string) {
	// Dangerous patterns to block
	dangerousPatterns := []struct {
		pattern string
		reason  string
	}{
		{`rm\s+(-[rf]*\s+)*\s*/`, "destructive filesystem operation on root"},
		{`:\(\)\{.*:\|:&\s*\};:`, "fork bomb pattern"},
		{`mkfs`, "filesystem formatting command"},
		{`dd\s+if=/dev/`, "dangerous dd command reading from device"},
		{`dd\s+of=/dev/`, "dangerous dd command writing to device"},
		{`>.*>/dev/[sh]d`, "direct write to block device"},
		{`chmod\s+-R\s+777\s+/`, "setting insecure permissions on root"},
		{`curl.*\|\s*bash`, "piping remote script directly to shell"},
		{`wget.*\|\s*bash`, "piping remote script directly to shell"},
		{`sudo\s+rm\s+-rf\s+/`, "destructive operation with sudo"},
		{`<<\s*['\"]?EOF`, "heredoc (cat <<EOF) - incompatible with command injection"},
		{`<<\s*['\"]?END`, "heredoc (cat <<END) - incompatible with command injection"},
		{`<<-`, "heredoc syntax - incompatible with command injection"},
	}

	cmdLower := strings.ToLower(command)

	for _, dp := range dangerousPatterns {
		matched, _ := regexp.MatchString(dp.pattern, cmdLower)
		if matched {
			return true, dp.reason
		}
	}

	return false, ""
}

// injectCommandIntoTerminal sends a command to a terminal widget for execution
func injectCommandIntoTerminal(ctx context.Context, fullBlockId string, command string) error {
	// Send command directly to the block controller
	commandWithNewline := command + "\n"
	inputData := []byte(commandWithNewline)

	inputUnion := &blockcontroller.BlockInputUnion{
		InputData: inputData,
	}

	return blockcontroller.SendInput(fullBlockId, inputUnion)
}

// monitorCommandCompletion polls the terminal until the command completes or timeout
func monitorCommandCompletion(ctx context.Context, fullBlockId string, timeoutSeconds int) (bool, int, error) {
	timeout := time.Duration(timeoutSeconds) * time.Second
	deadline := time.Now().Add(timeout)
	pollInterval := 200 * time.Millisecond

	blockORef := waveobj.MakeORef(waveobj.OType_Block, fullBlockId)

	// First, wait a moment for the input to be processed
	time.Sleep(100 * time.Millisecond)

	rtInfo := wstore.GetRTInfo(blockORef)
	if rtInfo == nil || !rtInfo.ShellIntegration {
		// No shell integration - can't monitor completion
		// Wait a reasonable time and assume it completed
		time.Sleep(3 * time.Second)
		return true, 0, nil
	}

	// CRITICAL: Wait for shell to START running the command (transition from ready to running-command)
	// This prevents detecting the PREVIOUS command's completion as this command's completion
	commandStarted := false
	startDeadline := time.Now().Add(10 * time.Second) // Give it 10 seconds to start

	for time.Now().Before(startDeadline) {
		select {
		case <-ctx.Done():
			return false, 0, ctx.Err()
		default:
		}

		rtInfo = wstore.GetRTInfo(blockORef)
		if rtInfo != nil && rtInfo.ShellState == "running-command" {
			commandStarted = true
			break
		}

		time.Sleep(pollInterval)
	}

	if !commandStarted {
		return false, 0, fmt.Errorf("command did not start within 10 seconds")
	}

	// Now wait for the command to COMPLETE (transition back to ready)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return false, 0, ctx.Err()
		default:
		}

		rtInfo = wstore.GetRTInfo(blockORef)
		if rtInfo == nil {
			return false, 0, fmt.Errorf("lost connection to terminal")
		}

		// Check if shell is ready (command completed)
		if rtInfo.ShellState == "ready" {
			exitCode := rtInfo.ShellLastCmdExitCode
			// Give the terminal a moment to finish updating
			time.Sleep(200 * time.Millisecond)
			return true, exitCode, nil
		}

		time.Sleep(pollInterval)
	}

	// Timeout reached
	return false, 0, fmt.Errorf("command execution timed out after %d seconds", timeoutSeconds)
}

func GetRunTerminalCommandToolDefinition(tabId string) uctypes.ToolDefinition {
	return uctypes.ToolDefinition{
		Name:        "run_terminal_command",
		DisplayName: "Run Terminal Command",
		Description: "Execute a shell command in a terminal widget. The command will be injected into the specified terminal and executed. Returns a summary of the command execution including exit code and output preview.",
		ToolLogName: "term:runcommand",
		Strict:      true,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"widget_id": map[string]any{
					"type":        "string",
					"description": "8-character widget ID of the terminal to use",
				},
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute",
				},
				"timeout_seconds": map[string]any{
					"type":        "number",
					"description": "Maximum time to wait for command completion (default: 300 seconds)",
				},
			},
			"required":             []string{"widget_id", "command", "timeout_seconds"},
			"additionalProperties": false,
		},
		ToolCallDesc: func(input any, output any, toolUseData *uctypes.UIMessageDataToolUse) string {
			parsed, err := parseRunTerminalCommandInput(input)
			if err != nil {
				return fmt.Sprintf("error parsing input: %v", err)
			}
			return fmt.Sprintf("running command: %s", utilfn.TruncateString(parsed.Command, 80))
		},
		ToolAnyCallback: func(input any, toolUseData *uctypes.UIMessageDataToolUse) (any, error) {
			parsed, err := parseRunTerminalCommandInput(input)
			if err != nil {
				return nil, err
			}

			// Safety check: block dangerous commands
			if dangerous, reason := isDangerousCommand(parsed.Command); dangerous {
				return nil, fmt.Errorf("command blocked for safety: %s", reason)
			}

			ctx, cancelFn := context.WithTimeout(context.Background(), time.Duration(*parsed.TimeoutSeconds+10)*time.Second)
			defer cancelFn()

			// Resolve full block ID
			fullBlockId, err := wcore.ResolveBlockIdFromPrefix(ctx, tabId, parsed.WidgetId)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve widget ID: %w", err)
			}

			// Verify it's a terminal block
			blockORef := waveobj.MakeORef(waveobj.OType_Block, fullBlockId)
			rtInfo := wstore.GetRTInfo(blockORef)
			if rtInfo == nil {
				return nil, fmt.Errorf("widget not found or not accessible")
			}

			// Check if terminal is currently running a command
			if rtInfo.ShellIntegration && rtInfo.ShellState == "running-command" {
				return nil, fmt.Errorf("terminal is currently running another command")
			}

			// Inject command into terminal
			startTime := time.Now()
			if err := injectCommandIntoTerminal(ctx, fullBlockId, parsed.Command); err != nil {
				return nil, fmt.Errorf("failed to inject command: %w", err)
			}

			// Monitor for completion
			completed, exitCode, err := monitorCommandCompletion(ctx, fullBlockId, *parsed.TimeoutSeconds)
			duration := time.Since(startTime)

			if err != nil {
				return map[string]any{
					"success":          false,
					"error":            err.Error(),
					"duration_seconds": duration.Seconds(),
				}, nil
			}

			if !completed {
				return map[string]any{
					"success":          false,
					"timeout":          true,
					"duration_seconds": duration.Seconds(),
					"message":          "Command did not complete within timeout",
				}, nil
			}

			// Get command output using term_get_scrollback
			var outputPreview string
			scrollbackOutput, err := getTermScrollbackOutput(tabId, parsed.WidgetId, wshrpc.CommandTermGetScrollbackLinesData{
				LastCommand: true,
			})
			if err == nil && scrollbackOutput != nil {
				outputPreview = scrollbackOutput.Content
				// Truncate to last 20 lines for summary
				lines := strings.Split(outputPreview, "\n")
				if len(lines) > 20 {
					outputPreview = strings.Join(lines[len(lines)-20:], "\n")
				}
			}

			return map[string]any{
				"success":          exitCode == 0,
				"exit_code":        exitCode,
				"duration_seconds": duration.Seconds(),
				"output_preview":   outputPreview,
				"message":          fmt.Sprintf("Command completed with exit code %d", exitCode),
			}, nil
		},
		ToolApproval: func(input any) string {
			// All terminal commands require approval
			return uctypes.ApprovalNeedsApproval
		},
	}
}
