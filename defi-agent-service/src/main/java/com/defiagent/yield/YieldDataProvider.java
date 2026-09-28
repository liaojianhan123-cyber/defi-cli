package com.defiagent.yield;

/**
 * Adapter over a single DeFi protocol's yield data.
 *
 * <p>The mock implementation returns deterministic demo data so the service runs
 * offline. To use real data, add an implementation backed by {@code RestClient}
 * calling e.g. DefiLlama / the protocol API — the rest of the pipeline is unchanged.
 */
public interface YieldDataProvider {

    String name();

    /**
     * Fetch the best opportunity for the given asset/chain, or {@code null} if
     * this provider has nothing for that pair.
     */
    YieldOpportunity fetch(String asset, String chain);
}
