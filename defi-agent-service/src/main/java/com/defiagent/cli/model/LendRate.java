package com.defiagent.cli.model;

/** Mirrors defi-cli's model.LendRate ("defi lend rates"). */
public record LendRate(
        String protocol,
        String provider,
        String chainId,
        String assetId,
        Double supplyApy,
        Double borrowApy,
        Double utilization,
        String sourceUrl,
        String fetchedAt
) {
}
