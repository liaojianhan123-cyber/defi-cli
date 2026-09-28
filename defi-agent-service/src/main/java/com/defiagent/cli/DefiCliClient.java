package com.defiagent.cli;

import com.defiagent.cli.model.ActionGasEstimate;
import com.defiagent.cli.model.BridgeQuote;
import com.defiagent.cli.model.Envelope;
import com.defiagent.cli.model.ErrorBody;
import com.defiagent.cli.model.ExecutionAction;
import com.defiagent.cli.model.GasPrice;
import com.defiagent.cli.model.LendMarket;
import com.defiagent.cli.model.LendPosition;
import com.defiagent.cli.model.LendRate;
import com.defiagent.cli.model.SwapQuote;
import com.defiagent.cli.model.WalletBalance;
import com.defiagent.cli.model.YieldOpportunity;
import com.defiagent.cli.model.YieldPosition;
import com.fasterxml.jackson.databind.JsonNode;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.type.CollectionType;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.stereotype.Component;

import java.util.ArrayList;
import java.util.List;

/**
 * Thin Java wrapper over the Go {@code defi-cli} binary. Each public method
 * maps 1:1 to a defi-cli command group (same argument names as defi-cli's own
 * Python agent tool dispatch in {@code agent/tools.py}, kept in sync
 * deliberately).
 *
 * <p><b>Boundary:</b> this client only ever invokes read-only queries and
 * {@code plan} / {@code show} / {@code estimate} commands. It never calls a
 * {@code submit} command, so it never signs or broadcasts a transaction —
 * that stays entirely out of the Java process.
 */
@Component
public class DefiCliClient {

    private static final Logger log = LoggerFactory.getLogger(DefiCliClient.class);

    private final CommandRunner runner;
    private final DefiCliProperties properties;
    private final ObjectMapper cliObjectMapper;

    public DefiCliClient(CommandRunner runner,
                          DefiCliProperties properties,
                          @Qualifier("defiCliObjectMapper") ObjectMapper cliObjectMapper) {
        this.runner = runner;
        this.properties = properties;
        this.cliObjectMapper = cliObjectMapper;
    }

    // ── Read-only queries ───────────────────────────────────────────────────

    public WalletBalance walletBalance(String chain, String address, String asset) {
        List<String> args = new ArrayList<>(List.of("wallet", "balance", "--chain", chain, "--address", address));
        addIfPresent(args, "--asset", asset);
        return runSingle(args, WalletBalance.class);
    }

    public List<LendMarket> lendMarkets(String provider, String chain, String asset) {
        List<String> args = new ArrayList<>(List.of("lend", "markets", "--provider", provider, "--chain", chain));
        addIfPresent(args, "--asset", asset);
        return runList(args, LendMarket.class);
    }

    public List<LendRate> lendRates(String provider, String chain, String asset) {
        List<String> args = new ArrayList<>(List.of("lend", "rates", "--provider", provider, "--chain", chain));
        addIfPresent(args, "--asset", asset);
        return runList(args, LendRate.class);
    }

    public List<LendPosition> lendPositions(String provider, String chain, String address, String type) {
        List<String> args = new ArrayList<>(
                List.of("lend", "positions", "--provider", provider, "--chain", chain, "--address", address));
        addIfPresent(args, "--type", type);
        return runList(args, LendPosition.class);
    }

    public List<YieldOpportunity> yieldOpportunities(String chain, String asset, String providers, Integer limit,
                                                      Double minTvlUsd, Double minApy, String sortBy) {
        List<String> args = new ArrayList<>(List.of("yield", "opportunities", "--chain", chain));
        addIfPresent(args, "--asset", asset);
        addIfPresent(args, "--providers", providers);
        if (limit != null) {
            args.add("--limit");
            args.add(String.valueOf(limit));
        }
        if (minTvlUsd != null) {
            args.add("--min-tvl-usd");
            args.add(String.valueOf(minTvlUsd));
        }
        if (minApy != null) {
            args.add("--min-apy");
            args.add(String.valueOf(minApy));
        }
        addIfPresent(args, "--sort", sortBy);
        return runList(args, YieldOpportunity.class);
    }

    public List<YieldPosition> yieldPositions(String chain, String address, String providers) {
        List<String> args = new ArrayList<>(List.of("yield", "positions", "--chain", chain, "--address", address));
        addIfPresent(args, "--providers", providers);
        return runList(args, YieldPosition.class);
    }

    public SwapQuote swapQuote(String provider, String chain, String fromAsset, String toAsset, String amount,
                                String fromAddress) {
        List<String> args = new ArrayList<>(List.of(
                "swap", "quote",
                "--provider", provider,
                "--chain", chain,
                "--from-asset", fromAsset,
                "--to-asset", toAsset,
                "--amount", amount));
        addIfPresent(args, "--from-address", fromAddress);
        return runSingle(args, SwapQuote.class);
    }

    public BridgeQuote bridgeQuote(String provider, String from, String to, String asset, String amount) {
        List<String> args = new ArrayList<>(List.of(
                "bridge", "quote",
                "--provider", provider,
                "--from", from,
                "--to", to,
                "--asset", asset,
                "--amount", amount));
        return runSingle(args, BridgeQuote.class);
    }

    public GasPrice gasPrice(String chain) {
        return runSingle(new ArrayList<>(List.of("chains", "gas", "--chain", chain)), GasPrice.class);
    }

    // ── Execution: plan / status only. Never submit/broadcast. ─────────────

    public ExecutionAction yieldDepositPlan(String provider, String chain, String asset, String amount,
                                             String fromAddress, String vaultAddress, String poolAddress) {
        List<String> args = new ArrayList<>(List.of(
                "yield", "deposit", "plan",
                "--provider", provider,
                "--chain", chain,
                "--asset", asset,
                "--amount", amount,
                "--from-address", fromAddress));
        addIfPresent(args, "--vault-address", vaultAddress);
        addIfPresent(args, "--pool-address", poolAddress);
        return runSingle(args, ExecutionAction.class);
    }

    public ExecutionAction lendSupplyPlan(String provider, String chain, String asset, String amount,
                                           String fromAddress, String poolAddress) {
        List<String> args = new ArrayList<>(List.of(
                "lend", "supply", "plan",
                "--provider", provider,
                "--chain", chain,
                "--asset", asset,
                "--amount", amount,
                "--from-address", fromAddress));
        addIfPresent(args, "--pool-address", poolAddress);
        return runSingle(args, ExecutionAction.class);
    }

    public ExecutionAction actionsShow(String actionId) {
        return runSingle(new ArrayList<>(List.of("actions", "show", "--action-id", actionId)), ExecutionAction.class);
    }

    public List<ExecutionAction> actionsList(Integer limit) {
        List<String> args = new ArrayList<>(List.of("actions", "list"));
        if (limit != null) {
            args.add("--limit");
            args.add(String.valueOf(limit));
        }
        return runList(args, ExecutionAction.class);
    }

    public ActionGasEstimate estimate(String actionId) {
        return runSingle(new ArrayList<>(List.of("actions", "estimate", "--action-id", actionId)), ActionGasEstimate.class);
    }

    // ── internals ────────────────────────────────────────────────────────

    private void addIfPresent(List<String> args, String flag, String value) {
        if (value != null && !value.isBlank()) {
            args.add(flag);
            args.add(value);
        }
    }

    private <T> T runSingle(List<String> args, Class<T> type) {
        Envelope envelope = execute(args);
        if (envelope.data() == null || envelope.data().isNull()) {
            return null;
        }
        return cliObjectMapper.convertValue(envelope.data(), type);
    }

    private <T> List<T> runList(List<String> args, Class<T> elementType) {
        Envelope envelope = execute(args);
        if (envelope.data() == null || envelope.data().isNull()) {
            return List.of();
        }
        CollectionType listType = cliObjectMapper.getTypeFactory().constructCollectionType(List.class, elementType);
        return cliObjectMapper.convertValue(envelope.data(), listType);
    }

    /**
     * {@code --results-only} (always appended by {@link DefiCliCommandRunner}) is
     * asymmetric: on <b>success</b> it prints the bare {@code data} payload (a raw
     * JSON array or object, no wrapper) — that's the whole point of the flag. On
     * <b>failure</b> it still prints the full {@code {version,success,data,error,meta}}
     * envelope, so the error code/type/message aren't lost. We detect which shape we
     * got by checking for the envelope's {@code success} field, rather than assuming
     * one shape always applies (a real defi-cli run surfaced this — none of it shows
     * up with a fake/mocked {@link CommandRunner} in tests).
     */
    private Envelope execute(List<String> args) {
        CommandResult result = runner.run(args, properties.getTimeout());
        JsonNode root;
        try {
            root = cliObjectMapper.readTree(result.output());
        } catch (Exception e) {
            log.error("failed to parse defi-cli output for args={} output={}", args, truncate(result.output()));
            throw DefiCliException.internal(
                    "failed to parse defi-cli JSON output (exit=" + result.exitCode() + ")", e);
        }

        if (root.isObject() && root.has("success")) {
            Envelope envelope = cliObjectMapper.convertValue(root, Envelope.class);
            if (!envelope.success()) {
                ErrorBody error = envelope.error();
                int code = error != null ? error.code() : result.exitCode();
                String type = error != null ? error.type() : "unknown";
                String message = error != null ? error.message() : "defi-cli command failed";
                throw new DefiCliException(code, type, message);
            }
            return envelope;
        }

        // Bare --results-only success payload (array or single object): synthesize
        // a successful envelope around it so runSingle/runList can stay unchanged.
        return new Envelope("v1", true, root, null, null);
    }

    private String truncate(String s) {
        if (s == null) {
            return "";
        }
        return s.length() > 500 ? s.substring(0, 500) + "..." : s;
    }
}
