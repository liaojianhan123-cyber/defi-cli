package com.defiagent.guardrails;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.util.List;

/**
 * Binds {@code agent.*} configuration — the Java equivalent of defi-cli's
 * Python agent config (agent/config.py). Override via env vars
 * (AGENT_MAX_SINGLE_TX_USD, AGENT_ALLOWED_PROTOCOLS, ...) or application.yml.
 */
@ConfigurationProperties(prefix = "agent")
public class AgentProperties {

    /** Hard cap on a single planned transaction's USD value. */
    private double maxSingleTxUsd = 10_000;

    /** Protocol allowlist (lowercase provider names); empty list = all allowed. */
    private List<String> allowedProtocols = List.of();

    /** APY percent above which a "may be a low-liquidity/risky pool" warning is attached. */
    private double highApyWarningThreshold = 50;

    /** Minimum pool TVL (USD) considered safe for a "large" deposit. */
    private double minTvlForLargeAmount = 1_000_000;

    /** USD threshold above which the low-TVL concentration-risk warning applies. */
    private double largeAmountUsd = 1_000;

    public double getMaxSingleTxUsd() {
        return maxSingleTxUsd;
    }

    public void setMaxSingleTxUsd(double maxSingleTxUsd) {
        this.maxSingleTxUsd = maxSingleTxUsd;
    }

    public List<String> getAllowedProtocols() {
        return allowedProtocols;
    }

    public void setAllowedProtocols(List<String> allowedProtocols) {
        this.allowedProtocols = allowedProtocols;
    }

    public double getHighApyWarningThreshold() {
        return highApyWarningThreshold;
    }

    public void setHighApyWarningThreshold(double highApyWarningThreshold) {
        this.highApyWarningThreshold = highApyWarningThreshold;
    }

    public double getMinTvlForLargeAmount() {
        return minTvlForLargeAmount;
    }

    public void setMinTvlForLargeAmount(double minTvlForLargeAmount) {
        this.minTvlForLargeAmount = minTvlForLargeAmount;
    }

    public double getLargeAmountUsd() {
        return largeAmountUsd;
    }

    public void setLargeAmountUsd(double largeAmountUsd) {
        this.largeAmountUsd = largeAmountUsd;
    }
}
