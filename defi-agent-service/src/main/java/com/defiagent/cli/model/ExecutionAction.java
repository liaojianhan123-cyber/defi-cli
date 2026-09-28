package com.defiagent.cli.model;

import java.util.List;

/**
 * Mirrors defi-cli's execution.Action — the result of a {@code plan} command
 * and the payload returned by {@code actions show}/{@code actions list}.
 *
 * <p>This service never calls the corresponding {@code submit} commands, so an
 * {@link ExecutionAction} here always reflects a planned (not broadcast) intent.
 */
public record ExecutionAction(
        String actionId,
        String intentType,
        String provider,
        String status,
        String chainId,
        String fromAddress,
        String toAddress,
        String inputAmount,
        String createdAt,
        String updatedAt,
        List<ActionStep> steps
) {
}
