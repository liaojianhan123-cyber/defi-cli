package com.defiagent.cli.model;

/**
 * Mirrors defi-cli's model.YieldOpportunity ("defi yield opportunities").
 *
 * <p>Distinct from {@link com.defiagent.yield.YieldOpportunity}, which is the
 * legacy mock-data record used by the demo/no-API-key chat fallback.
 */
public record YieldOpportunity(
        String opportunityId,
        String provider,
        String protocol,
        String chainId,
        String assetId,
        String type,
        Double apyBase,
        Double apyReward,
        Double apyTotal,
        Double tvlUsd,
        Double liquidityUsd,
        Double lockupDays,
        String withdrawalTerms,
        String sourceUrl,
        String fetchedAt
) {
}
