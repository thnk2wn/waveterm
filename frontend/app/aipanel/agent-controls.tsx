// Copyright 2025, Command Line Inc.
// SPDX-License-Identifier: Apache-2.0

// Agent mode controls for Wave AI
// This is a placeholder implementation for agent mode UI controls

import React from "react";

export interface AgentControlsProps {
    agentEnabled: boolean;
    currentIteration?: number;
    maxIterations?: number;
    onToggleAgent: (enabled: boolean) => void;
    onStopAgent: () => void;
}

export const AgentControls: React.FC<AgentControlsProps> = ({
    agentEnabled,
    currentIteration,
    maxIterations,
    onToggleAgent,
    onStopAgent,
}) => {
    return (
        <div className="agent-controls flex items-center gap-2 p-2 border-b border-zinc-700">
            <label className="flex items-center gap-2">
                <input
                    type="checkbox"
                    checked={agentEnabled}
                    onChange={(e) => onToggleAgent(e.target.checked)}
                    className="form-checkbox"
                />
                <span className="text-sm">Agent Mode</span>
            </label>

            {agentEnabled && currentIteration && (
                <div className="flex items-center gap-2 ml-auto">
                    <span className="text-xs text-zinc-400">
                        Iteration {currentIteration}/{maxIterations || 10}
                    </span>
                    <button
                        onClick={onStopAgent}
                        className="px-2 py-1 text-xs bg-red-600 hover:bg-red-700 rounded"
                    >
                        Stop
                    </button>
                </div>
            )}
        </div>
    );
};
