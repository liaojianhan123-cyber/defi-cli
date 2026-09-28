package com.defiagent.guardrails;

import org.springframework.stereotype.Service;

import java.util.ArrayList;
import java.util.List;
import java.util.Locale;

/**
 * Safety guardrails for execution planning — a direct Java port of defi-cli's
 * Python agent guardrails (agent/guardrails.py). All checks run BEFORE any
 * plan is created (i.e. before {@link com.defiagent.cli.DefiCliClient} is
 * ever called for a write action), so a blocked request never even reaches
 * defi-cli.
 */
@Service
public class GuardrailService {

    private final AgentProperties properties;

    public GuardrailService(AgentProperties properties) {
        this.properties = properties;
    }

    /** Rejects providers not in the allowlist (empty allowlist = all allowed). */
    public GuardrailResult checkProtocolAllowed(String provider) {
        List<String> allowed = properties.getAllowedProtocols();
        if (allowed == null || allowed.isEmpty()) {
            return GuardrailResult.allow();
        }
        String normalized = provider == null ? "" : provider.toLowerCase(Locale.ROOT);
        if (!allowed.contains(normalized)) {
            return GuardrailResult.blocked(String.format(
                    "Protocol '%s' is not in the allowed list: %s. Set agent.allowed-protocols (or AGENT_ALLOWED_PROTOCOLS) to expand it.",
                    provider, String.join(", ", allowed)));
        }
        return GuardrailResult.allow();
    }

    /** Blocks transactions above the configured maximum. */
    public GuardrailResult checkAmountUsd(double amountUsd) {
        if (amountUsd > properties.getMaxSingleTxUsd()) {
            return GuardrailResult.blocked(String.format(
                    "Transaction amount $%,.2f exceeds the agent limit of $%,.2f. Increase agent.max-single-tx-usd or split the transaction.",
                    amountUsd, properties.getMaxSingleTxUsd()));
        }
        return GuardrailResult.allow();
    }

    /** Non-blocking warning when APY looks suspiciously high. */
    public String warnHighApy(double apy) {
        if (apy > properties.getHighApyWarningThreshold()) {
            return String.format(
                    "APY %.1f%% is unusually high (>%.0f%%). This may indicate a new or low-liquidity pool with elevated risk.",
                    apy, properties.getHighApyWarningThreshold());
        }
        return null;
    }

    /** Non-blocking warning when TVL is low relative to the deposit size. */
    public String warnLowTvl(double tvlUsd, double amountUsd) {
        if (amountUsd >= properties.getLargeAmountUsd() && tvlUsd < properties.getMinTvlForLargeAmount()) {
            return String.format(
                    "Pool TVL is $%,.0f, which is low for a $%,.0f deposit. Consider splitting across multiple protocols to reduce concentration risk.",
                    tvlUsd, amountUsd);
        }
        return null;
    }

    /**
     * Runs all guardrails and returns a combined issue list. Blocking issues are
     * prefixed {@code BLOCK:}, warnings {@code WARN:} — mirrors
     * {@code guardrails.run_all} in the Python agent.
     */
    public List<String> runAll(String provider, double amountUsd, double apy, double tvlUsd) {
        List<String> issues = new ArrayList<>();

        GuardrailResult protocolResult = checkProtocolAllowed(provider);
        if (!protocolResult.allowed()) {
            issues.add("BLOCK: " + protocolResult.reason());
        }

        if (amountUsd > 0) {
            GuardrailResult amountResult = checkAmountUsd(amountUsd);
            if (!amountResult.allowed()) {
                issues.add("BLOCK: " + amountResult.reason());
            }
        }

        if (apy > 0) {
            String warn = warnHighApy(apy);
            if (warn != null) {
                issues.add("WARN: " + warn);
            }
        }

        if (tvlUsd > 0 && amountUsd > 0) {
            String warn = warnLowTvl(tvlUsd, amountUsd);
            if (warn != null) {
                issues.add("WARN: " + warn);
            }
        }

        return issues;
    }
}
