package com.defiagent.cli;

import com.defiagent.cli.model.LendMarket;
import com.defiagent.cli.model.WalletBalance;
import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.PropertyNamingStrategies;
import org.junit.jupiter.api.Test;

import java.time.Duration;
import java.util.List;

import static org.assertj.core.api.Assertions.assertThat;
import static org.junit.jupiter.api.Assertions.assertThrows;

/**
 * Uses a {@link FakeCommandRunner} instead of a real defi-cli binary, so this
 * test verifies argument construction + Envelope parsing + error mapping
 * without any process/network dependency.
 */
class DefiCliClientTest {

    private final ObjectMapper mapper = new ObjectMapper()
            .setPropertyNamingStrategy(PropertyNamingStrategies.SNAKE_CASE)
            .configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false);

    @Test
    void walletBalance_parsesSingleObjectAndBuildsExpectedArgs() {
        FakeCommandRunner runner = new FakeCommandRunner("""
                {
                  "version": "1",
                  "success": true,
                  "data": {
                    "chain_id": "eip155:1",
                    "account_address": "0xabc",
                    "asset_type": "native",
                    "asset_id": "eip155:1/slip44:60",
                    "symbol": "ETH",
                    "balance": {"amount_base_units": "1000000000000000000", "amount_decimal": "1", "decimals": 18},
                    "fetched_at": "2026-01-01T00:00:00Z"
                  }
                }
                """);
        DefiCliClient client = new DefiCliClient(runner, new DefiCliProperties(), mapper);

        WalletBalance balance = client.walletBalance("ethereum", "0xabc", null);

        assertThat(balance.symbol()).isEqualTo("ETH");
        assertThat(balance.balance().amountDecimal()).isEqualTo("1");
        assertThat(runner.lastArgs()).containsExactly("wallet", "balance", "--chain", "ethereum", "--address", "0xabc");
    }

    @Test
    void lendMarkets_parsesArrayAndAppendsOptionalAssetFlag() {
        FakeCommandRunner runner = new FakeCommandRunner("""
                {
                  "version": "1",
                  "success": true,
                  "data": [
                    {"protocol": "aave-v3", "provider": "aave", "chain_id": "eip155:1", "asset_id": "usdc",
                     "supply_apy": 4.5, "borrow_apy": 6.1, "tvl_usd": 1000000, "liquidity_usd": 500000}
                  ]
                }
                """);
        DefiCliClient client = new DefiCliClient(runner, new DefiCliProperties(), mapper);

        List<LendMarket> markets = client.lendMarkets("aave", "ethereum", "USDC");

        assertThat(markets).hasSize(1);
        assertThat(markets.get(0).supplyApy()).isEqualTo(4.5);
        assertThat(runner.lastArgs()).containsExactly(
                "lend", "markets", "--provider", "aave", "--chain", "ethereum", "--asset", "USDC");
    }

    @Test
    void emptyDataArray_returnsEmptyListNotNull() {
        FakeCommandRunner runner = new FakeCommandRunner("""
                {"version": "1", "success": true, "data": []}
                """);
        DefiCliClient client = new DefiCliClient(runner, new DefiCliProperties(), mapper);

        assertThat(client.lendMarkets("aave", "ethereum", "USDC")).isEmpty();
    }

    @Test
    void errorEnvelope_throwsDefiCliExceptionWithCodeAndMessage() {
        FakeCommandRunner runner = new FakeCommandRunner("""
                {
                  "version": "1",
                  "success": false,
                  "error": {"code": 12, "type": "unavailable", "message": "rpc down"}
                }
                """);
        DefiCliClient client = new DefiCliClient(runner, new DefiCliProperties(), mapper);

        DefiCliException ex = assertThrows(DefiCliException.class,
                () -> client.walletBalance("ethereum", "0xabc", null));

        assertThat(ex.getCode()).isEqualTo(12);
        assertThat(ex.getType()).isEqualTo("unavailable");
        assertThat(ex.getMessage()).isEqualTo("rpc down");
    }

    @Test
    void bareArrayPayload_resultsOnlySuccessShapeWithNoEnvelopeWrapper() {
        // --results-only prints the bare `data` payload on success, with no
        // {version,success,data} wrapper at all — this is what a real defi-cli
        // binary actually returns (a fake/mocked runner alone wouldn't catch this).
        FakeCommandRunner runner = new FakeCommandRunner("""
                [
                  {"protocol": "aave-v3", "provider": "aave", "chain_id": "eip155:1", "asset_id": "usdc",
                   "supply_apy": 4.5, "borrow_apy": 6.1, "tvl_usd": 1000000, "liquidity_usd": 500000}
                ]
                """);
        DefiCliClient client = new DefiCliClient(runner, new DefiCliProperties(), mapper);

        List<LendMarket> markets = client.lendMarkets("aave", "ethereum", "USDC");

        assertThat(markets).hasSize(1);
        assertThat(markets.get(0).supplyApy()).isEqualTo(4.5);
    }

    @Test
    void bareObjectPayload_resultsOnlySuccessShapeWithNoEnvelopeWrapper() {
        FakeCommandRunner runner = new FakeCommandRunner("""
                {
                  "chain_id": "eip155:1",
                  "account_address": "0xabc",
                  "asset_type": "native",
                  "asset_id": "eip155:1/slip44:60",
                  "symbol": "ETH",
                  "balance": {"amount_base_units": "1000000000000000000", "amount_decimal": "1", "decimals": 18},
                  "fetched_at": "2026-01-01T00:00:00Z"
                }
                """);
        DefiCliClient client = new DefiCliClient(runner, new DefiCliProperties(), mapper);

        WalletBalance balance = client.walletBalance("ethereum", "0xabc", null);

        assertThat(balance.symbol()).isEqualTo("ETH");
    }

    @Test
    void errorEnvelope_stillDetectedEvenThoughSuccessPayloadHasNoWrapper() {
        // On failure, --results-only still prints the FULL envelope (so the error
        // code/message survives) — this must keep working after bare-payload support.
        FakeCommandRunner runner = new FakeCommandRunner("""
                {
                  "version": "v1",
                  "success": false,
                  "data": [],
                  "error": {"code": 2, "type": "usage_error", "message": "required flag \\"asset\\" not set"}
                }
                """);
        DefiCliClient client = new DefiCliClient(runner, new DefiCliProperties(), mapper);

        DefiCliException ex = assertThrows(DefiCliException.class,
                () -> client.lendMarkets("notreal", "ethereum", null));

        assertThat(ex.getCode()).isEqualTo(2);
        assertThat(ex.getType()).isEqualTo("usage_error");
    }

    @Test
    void malformedOutput_throwsInternalDefiCliException() {
        FakeCommandRunner runner = new FakeCommandRunner("not json");
        DefiCliClient client = new DefiCliClient(runner, new DefiCliProperties(), mapper);

        DefiCliException ex = assertThrows(DefiCliException.class,
                () -> client.walletBalance("ethereum", "0xabc", null));

        assertThat(ex.getType()).isEqualTo("internal");
    }

    private static final class FakeCommandRunner implements CommandRunner {
        private final String output;
        private List<String> lastArgs;

        private FakeCommandRunner(String output) {
            this.output = output;
        }

        @Override
        public CommandResult run(List<String> args, Duration timeout) {
            this.lastArgs = args;
            return new CommandResult(0, output);
        }

        List<String> lastArgs() {
            return lastArgs;
        }
    }
}
