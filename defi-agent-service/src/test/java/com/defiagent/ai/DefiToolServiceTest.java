package com.defiagent.ai;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.guardrails.AgentProperties;
import com.defiagent.guardrails.GuardrailService;
import org.junit.jupiter.api.Test;

import java.util.List;
import java.util.Map;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;

/** Verifies the guardrail-interception path: a blocked protocol never reaches {@link DefiCliClient}. */
class DefiToolServiceTest {

    @Test
    void yieldDepositPlan_blockedProtocol_neverCallsCli() {
        AgentProperties properties = new AgentProperties();
        properties.setAllowedProtocols(List.of("aave"));
        GuardrailService guardrails = new GuardrailService(properties);
        DefiCliClient cli = mock(DefiCliClient.class);
        DefiToolService toolService = new DefiToolService(cli, guardrails, mock(com.defiagent.repo.AgentActionRepository.class));

        Object result = toolService.yieldDepositPlan("compoundv3", "ethereum", "USDC", "1000000", "0xabc", null, null, null);

        assertThat(result).isInstanceOf(Map.class);
        assertThat(((Map<?, ?>) result).get("error").toString()).contains("compoundv3");
        verify(cli, never()).yieldDepositPlan(any(), any(), any(), any(), any(), any(), any());
    }

    @Test
    void yieldDepositPlan_allowedProtocol_delegatesToCli() {
        AgentProperties properties = new AgentProperties(); // empty allowlist = allow all
        GuardrailService guardrails = new GuardrailService(properties);
        DefiCliClient cli = mock(DefiCliClient.class);
        DefiToolService toolService = new DefiToolService(cli, guardrails, mock(com.defiagent.repo.AgentActionRepository.class));

        toolService.yieldDepositPlan("aave", "ethereum", "USDC", "1000000", "0xabc", null, null, null);

        verify(cli).yieldDepositPlan("aave", "ethereum", "USDC", "1000000", "0xabc", null, null);
    }

    @Test
    void lendSupplyPlan_blockedProtocol_neverCallsCli() {
        AgentProperties properties = new AgentProperties();
        properties.setAllowedProtocols(List.of("aave"));
        GuardrailService guardrails = new GuardrailService(properties);
        DefiCliClient cli = mock(DefiCliClient.class);
        DefiToolService toolService = new DefiToolService(cli, guardrails, mock(com.defiagent.repo.AgentActionRepository.class));

        Object result = toolService.lendSupplyPlan("morpho", "ethereum", "USDC", "1000000", "0xabc", null, null);

        assertThat(((Map<?, ?>) result).get("error").toString()).contains("morpho");
        verify(cli, never()).lendSupplyPlan(any(), any(), any(), any(), any(), any());
    }

    @Test
    void yieldOpportunities_delegatesToCliWithDefaultLimit() {
        DefiCliClient cli = mock(DefiCliClient.class);
        DefiToolService toolService = new DefiToolService(cli, new GuardrailService(new AgentProperties()), mock(com.defiagent.repo.AgentActionRepository.class));

        toolService.yieldOpportunities("ethereum", "USDC", null, null, null, null);

        verify(cli).yieldOpportunities("ethereum", "USDC", null, 10, null, null, "apy_total");
    }
}
