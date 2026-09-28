package com.defiagent.cli.model;

/** Mirrors defi-cli's execution.ActionGasEstimateChainTotal. */
public record ActionGasEstimateChainTotal(
        String chainId,
        String likelyFeeWei,
        String worstCaseFeeWei,
        String feeUnit,
        String feeToken
) {
}
