package com.defiagent.cli.model;

import java.util.List;

/** Mirrors defi-cli's execution.ActionStep. */
public record ActionStep(
        String stepId,
        String type,
        String status,
        String chainId,
        String description,
        String target,
        String data,
        String value,
        String txHash,
        String error,
        List<StepCall> calls
) {
}
