package com.defiagent.cli.model;

/** Mirrors defi-cli's model.BridgeQuote ("defi bridge quote"). */
public record BridgeQuote(
        String provider,
        String fromChainId,
        String toChainId,
        String fromAssetId,
        String toAssetId,
        AmountInfo inputAmount,
        AmountInfo estimatedOut,
        Double estimatedFeeUsd,
        Long estimatedTimeS,
        String route,
        String sourceUrl,
        String fetchedAt
) {
}
