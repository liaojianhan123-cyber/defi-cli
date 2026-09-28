package com.defiagent.guardrails;

import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

/** Java port of defi-cli's Python test_guardrails.py cases. */
class GuardrailServiceTest {

    private AgentProperties properties;
    private GuardrailService service;

    @BeforeEach
    void setUp() {
        properties = new AgentProperties();
        service = new GuardrailService(properties);
    }

    @Test
    void checkProtocolAllowed_allowsEverythingWhenAllowlistEmpty() {
        assertThat(service.checkProtocolAllowed("anything").allowed()).isTrue();
    }

    @Test
    void checkProtocolAllowed_blocksProtocolNotInAllowlist() {
        properties.setAllowedProtocols(List.of("aave", "morpho"));

        GuardrailResult result = service.checkProtocolAllowed("compoundv3");

        assertThat(result.allowed()).isFalse();
        assertThat(result.reason()).contains("compoundv3", "aave", "morpho");
    }

    @Test
    void checkProtocolAllowed_allowsProtocolInAllowlistCaseInsensitive() {
        properties.setAllowedProtocols(List.of("aave"));

        assertThat(service.checkProtocolAllowed("AAVE").allowed()).isTrue();
    }

    @Test
    void checkAmountUsd_blocksAboveLimit() {
        properties.setMaxSingleTxUsd(1000);

        GuardrailResult result = service.checkAmountUsd(1500);

        assertThat(result.allowed()).isFalse();
        assertThat(result.reason()).contains("1,500").contains("1,000");
    }

    @Test
    void checkAmountUsd_allowsAtOrBelowLimit() {
        properties.setMaxSingleTxUsd(1000);

        assertThat(service.checkAmountUsd(1000).allowed()).isTrue();
    }

    @Test
    void warnHighApy_returnsWarningOnlyAboveThreshold() {
        properties.setHighApyWarningThreshold(50);

        assertThat(service.warnHighApy(80)).contains("80.0%");
        assertThat(service.warnHighApy(20)).isNull();
    }

    @Test
    void warnLowTvl_onlyWarnsForLargeAmountsWithLowTvl() {
        properties.setLargeAmountUsd(1000);
        properties.setMinTvlForLargeAmount(1_000_000);

        assertThat(service.warnLowTvl(500_000, 2000)).isNotNull();
        assertThat(service.warnLowTvl(500_000, 500)).isNull();
        assertThat(service.warnLowTvl(2_000_000, 2000)).isNull();
    }

    @Test
    void runAll_combinesBlockAndWarnIssuesWithPrefixes() {
        properties.setAllowedProtocols(List.of("aave"));
        properties.setMaxSingleTxUsd(1000);
        properties.setHighApyWarningThreshold(50);

        List<String> issues = service.runAll("compoundv3", 2000, 80, 100);

        assertThat(issues).anySatisfy(issue -> assertThat(issue).startsWith("BLOCK:").contains("compoundv3"));
        assertThat(issues).anySatisfy(issue -> assertThat(issue).startsWith("BLOCK:").contains("2,000"));
        assertThat(issues).anySatisfy(issue -> assertThat(issue).startsWith("WARN:").contains("80"));
    }
}
