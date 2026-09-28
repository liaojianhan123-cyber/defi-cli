package com.defiagent.cli.model;

/** Mirrors defi-cli's model.SwapQuote ("defi swap quote"). */
public record SwapQuote(
        String provider,
        String chainId,
        String fromAssetId,
        String toAssetId,
        String tradeType,
        AmountInfo inputAmount,
        AmountInfo estimatedOut,
        Double estimatedGasUsd,
        Double priceImpactPct,
        String route,
        String sourceUrl,
        String fetchedAt
) {
}
