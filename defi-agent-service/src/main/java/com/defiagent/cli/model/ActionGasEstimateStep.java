package com.defiagent.cli.model;

/** Mirrors defi-cli's execution.ActionGasEstimateStep ("defi actions estimate"). */
public record ActionGasEstimateStep(
        String stepId,
        String type,
        String status,
        String chainId,
        String gasEstimateRaw,
        String gasLimit,
        String likelyFeeWei,
        String worstCaseFeeWei,
        String feeUnit,
        String feeToken
) {
}
