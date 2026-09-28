package com.defiagent.cli.model;

/** Mirrors defi-cli's model.LendPosition ("defi lend positions"). */
public record LendPosition(
        String protocol,
        String provider,
        String chainId,
        String accountAddress,
        String positionType,
        String assetId,
        AmountInfo amount,
        Double amountUsd,
        Double apy,
        String sourceUrl,
        String fetchedAt
) {
}
