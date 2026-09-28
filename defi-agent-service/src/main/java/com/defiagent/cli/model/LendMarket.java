package com.defiagent.cli.model;

/** Mirrors defi-cli's model.LendMarket ("defi lend markets"). */
public record LendMarket(
        String protocol,
        String provider,
        String chainId,
        String assetId,
        Double supplyApy,
        Double borrowApy,
        Double tvlUsd,
        Double liquidityUsd,
        String sourceUrl,
        String fetchedAt
) {
}
