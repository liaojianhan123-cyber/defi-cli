package com.defiagent.yield;

/**
 * A normalized yield opportunity across protocols.
 * apy is in percentage points (4.5 == 4.5%).
 */
public record YieldOpportunity(
        String provider,
        String chain,
        String asset,
        double apy,
        double tvlUsd
) {
}
