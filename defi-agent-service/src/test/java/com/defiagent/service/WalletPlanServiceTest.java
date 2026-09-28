package com.defiagent.service;

import com.defiagent.cli.model.ActionStep;
import com.defiagent.cli.model.ExecutionAction;
import com.defiagent.cli.model.StepCall;
import com.defiagent.web.dto.UnsignedTx;
import org.junit.jupiter.api.Test;

import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;

class WalletPlanServiceTest {

    @Test
    void toChainIdHex_caip2EthereumMainnet() {
        assertThat(WalletPlanService.toChainIdHex("eip155:1")).isEqualTo("0x1");
    }

    @Test
    void toValueHex_decimalWei() {
        assertThat(WalletPlanService.toValueHex("0")).isEqualTo("0x0");
        assertThat(WalletPlanService.toValueHex("255")).isEqualTo("0xff");
    }

    @Test
    void flatten_usesNestedCallsThenFallsBackToStepFields() {
        ActionStep batched = new ActionStep(
                "s1", "approval", "pending", "eip155:1", "approve USDC",
                null, null, "0", null, null,
                List.of(new StepCall("0xToken", "0x095ea7b3", "0")));
        ActionStep single = new ActionStep(
                "s2", "lend_call", "pending", "eip155:1", "supply",
                "0xPool", "0xdead", "0", null, null, null);
        ExecutionAction action = new ExecutionAction(
                "cli-1", "deposit", "aave", "planned", "eip155:1",
                "0xabc", "0xPool", "1000", null, null, List.of(batched, single));

        List<UnsignedTx> txs = WalletPlanService.flatten(action);

        assertThat(txs).hasSize(2);
        assertThat(txs.get(0).to()).isEqualTo("0xToken");
        assertThat(txs.get(0).data()).isEqualTo("0x095ea7b3");
        assertThat(txs.get(0).chainIdHex()).isEqualTo("0x1");
        assertThat(txs.get(1).to()).isEqualTo("0xPool");
        assertThat(txs.get(1).from()).isEqualTo("0xabc");
    }
}
