package com.defiagent.yield;

/**
 * Deterministic mock provider so demos run without network / API keys.
 * apy/tvl are derived from a base value + a stable hash of asset/chain so the
 * numbers look realistic and vary per input, but are reproducible.
 */
public class MockYieldProvider implements YieldDataProvider {

    private final String name;
    private final double baseApy;
    private final double baseTvlUsd;

    public MockYieldProvider(String name, double baseApy, double baseTvlUsd) {
        this.name = name;
        this.baseApy = baseApy;
        this.baseTvlUsd = baseTvlUsd;
    }

    @Override
    public String name() {
        return name;
    }

    @Override
    public YieldOpportunity fetch(String asset, String chain) {
        int jitter = Math.floorMod((asset + ":" + chain + ":" + name).hashCode(), 100);
        double apy = round2(baseApy + jitter / 50.0);          // e.g. 3.2% .. 5.2%
        double tvl = baseTvlUsd * (1.0 + jitter / 200.0);      // slight variation
        return new YieldOpportunity(name, chain, asset, apy, Math.round(tvl));
    }

    private static double round2(double v) {
        return Math.round(v * 100.0) / 100.0;
    }
}
