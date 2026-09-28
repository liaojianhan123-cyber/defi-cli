package com.defiagent.cli.model;

/** Mirrors defi-cli's model.YieldPosition ("defi yield positions"). */
public record YieldPosition(
        String protocol,
        String provider,
        String chainId,
        String accountAddress,
        String positionType,
        String opportunityId,
        String assetId,
        AmountInfo amount,
        Double amountUsd,
        Double apyTotal,
        String sourceUrl,
        String fetchedAt
) {
}
