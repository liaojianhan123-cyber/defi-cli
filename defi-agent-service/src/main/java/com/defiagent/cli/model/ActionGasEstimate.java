package com.defiagent.cli.model;

import java.util.List;

/**
 * Mirrors defi-cli's execution.ActionGasEstimate ("defi actions estimate").
 * Real gas/fee estimate for a planned action — read-only, no signing involved.
 */
public record ActionGasEstimate(
        String actionId,
        String estimatedAt,
        String blockTag,
        List<ActionGasEstimateStep> steps,
        List<ActionGasEstimateChainTotal> totalsByChain
) {
}
