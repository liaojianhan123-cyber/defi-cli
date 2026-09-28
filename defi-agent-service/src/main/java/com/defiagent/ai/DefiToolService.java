package com.defiagent.ai;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.cli.model.ActionStep;
import com.defiagent.cli.model.ExecutionAction;
import com.defiagent.cli.model.GasPrice;
import com.defiagent.cli.model.LendMarket;
import com.defiagent.cli.model.LendPosition;
import com.defiagent.cli.model.LendRate;
import com.defiagent.cli.model.SwapQuote;
import com.defiagent.cli.model.WalletBalance;
import com.defiagent.cli.model.YieldOpportunity;
import com.defiagent.cli.model.YieldPosition;
import com.defiagent.domain.ActionStatus;
import com.defiagent.domain.AgentAction;
import com.defiagent.guardrails.GuardrailResult;
import com.defiagent.guardrails.GuardrailService;
import com.defiagent.repo.AgentActionRepository;
import com.defiagent.web.SessionContext;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.ai.chat.model.ToolContext;
import org.springframework.ai.tool.annotation.Tool;
import org.springframework.ai.tool.annotation.ToolParam;
import org.springframework.stereotype.Component;

import java.util.List;
import java.util.Map;
import java.util.UUID;

/**
 * Tool surface exposed to the LLM via Spring AI. Mirrors defi-cli's own
 * Python agent tool schema (defi-cli/agent/tools.py) 1:1 so the orchestration
 * contract stays consistent whichever language drives it.
 *
 * <p><b>Execution boundary:</b> only {@code *Plan} tools are exposed here, and
 * each runs {@link GuardrailService} checks before calling defi-cli. There is
 * intentionally NO {@code submit}/broadcast tool — unlike the Python agent
 * (which gates submit tools behind an interactive terminal confirmation), this
 * Java service simply never gives the model the capability to broadcast.
 * The human confirms in the browser wallet (MetaMask); paper Kafka execution
 * remains a demo fallback when no wallet is used.
 */
@Component
public class DefiToolService {

    private static final Logger log = LoggerFactory.getLogger(DefiToolService.class);

    private final DefiCliClient cli;
    private final GuardrailService guardrails;
    private final AgentActionRepository actionRepository;

    public DefiToolService(DefiCliClient cli, GuardrailService guardrails, AgentActionRepository actionRepository) {
        this.cli = cli;
        this.guardrails = guardrails;
        this.actionRepository = actionRepository;
    }

    // ── Read-only ────────────────────────────────────────────────────────

    @Tool(description = "Find yield opportunities (lending pools, AMM pools) across DeFi protocols. "
            + "Returns APY breakdown (base + reward + total), TVL, liquidity, lockup days, and withdrawal terms. "
            + "Use this to compare protocols before recommending one.")
    public List<YieldOpportunity> yieldOpportunities(
            @ToolParam(description = "Chain name or ID, e.g. '1', 'ethereum', 'base', 'arbitrum'") String chain,
            @ToolParam(description = "Asset symbol filter, e.g. 'USDC', 'ETH'. Omit for all assets.", required = false) String asset,
            @ToolParam(description = "Comma-separated provider filter, e.g. 'aave,morpho'. Omit for all.", required = false) String providers,
            @ToolParam(description = "Max results, default 10", required = false) Integer limit,
            @ToolParam(description = "Minimum TVL in USD to filter out tiny pools", required = false) Double minTvlUsd,
            @ToolParam(description = "Minimum total APY percent", required = false) Double minApy) {
        log.info("tool call: yieldOpportunities chain={} asset={} providers={}", chain, asset, providers);
        return cli.yieldOpportunities(chain, asset, providers, limit == null ? 10 : limit, minTvlUsd, minApy, "apy_total");
    }

    @Tool(description = "Get lending market data (supply APY, borrow APY, TVL) for a specific lending protocol.")
    public List<LendMarket> lendMarkets(
            @ToolParam(description = "Provider: aave | morpho | compoundv3 | moonwell | kamino") String provider,
            @ToolParam(description = "Chain name or ID") String chain,
            @ToolParam(description = "Asset symbol filter", required = false) String asset) {
        return cli.lendMarkets(provider, chain, asset);
    }

    @Tool(description = "Get current supply/borrow rates and utilization for a lending protocol.")
    public List<LendRate> lendRates(
            @ToolParam(description = "Provider: aave | morpho | compoundv3 | moonwell | kamino") String provider,
            @ToolParam(description = "Chain name or ID") String chain,
            @ToolParam(description = "Asset symbol filter", required = false) String asset) {
        return cli.lendRates(provider, chain, asset);
    }

    @Tool(description = "Check current yield positions (deposits, LP holdings) for a wallet address.")
    public List<YieldPosition> yieldPositions(
            @ToolParam(description = "Chain name or ID") String chain,
            @ToolParam(description = "EVM wallet address (0x...)") String address,
            @ToolParam(description = "Comma-separated providers. Omit for all.", required = false) String providers) {
        return cli.yieldPositions(chain, address, providers);
    }

    @Tool(description = "Check current lending/borrowing positions for a wallet address.")
    public List<LendPosition> lendPositions(
            @ToolParam(description = "Provider: aave | morpho | compoundv3 | moonwell") String provider,
            @ToolParam(description = "Chain name or ID") String chain,
            @ToolParam(description = "EVM wallet address (0x...)") String address,
            @ToolParam(description = "Position type filter: all | supply | borrow | collateral", required = false) String type) {
        return cli.lendPositions(provider, chain, address, type == null ? "all" : type);
    }

    @Tool(description = "Get a swap quote between two assets. Read-only, does not execute anything.")
    public SwapQuote swapQuote(
            @ToolParam(description = "Swap provider: uniswap | tempo | taikoswap") String provider,
            @ToolParam(description = "Chain name or ID") String chain,
            @ToolParam(description = "Input token symbol or address") String fromAsset,
            @ToolParam(description = "Output token symbol or address") String toAsset,
            @ToolParam(description = "Input amount in base units, e.g. '1000000' for 1 USDC (6 decimals)") String amount,
            @ToolParam(description = "Sender address, required for Uniswap", required = false) String fromAddress) {
        return cli.swapQuote(provider, chain, fromAsset, toAsset, amount, fromAddress);
    }

    @Tool(description = "Query native or ERC-20 token balance for a wallet address on an EVM chain.")
    public WalletBalance walletBalance(
            @ToolParam(description = "Chain name or ID") String chain,
            @ToolParam(description = "EVM wallet address") String address,
            @ToolParam(description = "ERC-20 token symbol/address filter; omit for native balance", required = false) String asset) {
        return cli.walletBalance(chain, address, asset);
    }

    @Tool(description = "Get current gas prices (base fee, priority fee) for a chain.")
    public GasPrice gasPrice(@ToolParam(description = "Chain name or ID") String chain) {
        return cli.gasPrice(chain);
    }

    // ── Execution: plan only. Never broadcasts. ─────────────────────────

    @Tool(description = "Plan a yield deposit. Creates a real defi-cli action plan (action_id + on-chain call steps). "
            + "Does NOT broadcast anything — the user must confirm in MetaMask on the UI. "
            + "Use after choosing a protocol from yieldOpportunities.")
    public Object yieldDepositPlan(
            @ToolParam(description = "Provider: aave | morpho | compoundv3 | moonwell") String provider,
            @ToolParam(description = "Chain name or ID") String chain,
            @ToolParam(description = "Asset to deposit, e.g. 'USDC'") String asset,
            @ToolParam(description = "Amount in base units, e.g. '1000000' = 1 USDC with 6 decimals") String amount,
            @ToolParam(description = "Sender wallet address — must be the user's MetaMask account") String fromAddress,
            @ToolParam(description = "Morpho vault address (required for morpho)", required = false) String vaultAddress,
            @ToolParam(description = "Explicit pool/Comet address override", required = false) String poolAddress,
            ToolContext toolContext) {
        GuardrailResult check = guardrails.checkProtocolAllowed(provider);
        if (!check.allowed()) {
            log.warn("yieldDepositPlan blocked by guardrail: {}", check.reason());
            return Map.of("error", check.reason());
        }
        ExecutionAction planned = cli.yieldDepositPlan(provider, chain, asset, amount, fromAddress, vaultAddress, poolAddress);
        persistPlanned(planned, "DEPOSIT", provider, chain, asset, amount, toolContext);
        return planned;
    }

    @Tool(description = "Plan a lending supply. Creates a real defi-cli action plan, does NOT broadcast. "
            + "Returns action_id + steps for the user to review and sign in MetaMask.")
    public Object lendSupplyPlan(
            @ToolParam(description = "Provider: aave | morpho | compoundv3 | moonwell") String provider,
            @ToolParam(description = "Chain name or ID") String chain,
            @ToolParam(description = "Asset to supply") String asset,
            @ToolParam(description = "Amount in base units") String amount,
            @ToolParam(description = "Sender wallet address — must be the user's MetaMask account") String fromAddress,
            @ToolParam(description = "Optional pool address override", required = false) String poolAddress,
            ToolContext toolContext) {
        GuardrailResult check = guardrails.checkProtocolAllowed(provider);
        if (!check.allowed()) {
            log.warn("lendSupplyPlan blocked by guardrail: {}", check.reason());
            return Map.of("error", check.reason());
        }
        ExecutionAction planned = cli.lendSupplyPlan(provider, chain, asset, amount, fromAddress, poolAddress);
        persistPlanned(planned, "DEPOSIT", provider, chain, asset, amount, toolContext);
        return planned;
    }

    private void persistPlanned(ExecutionAction planned, String intent, String provider, String chain,
                                String asset, String amount, ToolContext toolContext) {
        String session = sessionIdFrom(toolContext);
        if (session == null || session.isBlank() || planned == null || planned.actionId() == null) {
            log.warn("skip persist planned action: session={} defiActionId={}", session,
                    planned == null ? null : planned.actionId());
            return;
        }
        try {
            AgentAction row = new AgentAction();
            row.setActionId(UUID.randomUUID().toString());
            row.setSessionId(session);
            row.setIntent(intent);
            row.setProvider(provider);
            row.setChain(chain);
            row.setAsset(asset);
            row.setAmount(amount);
            row.setStatus(ActionStatus.PLANNED);
            row.setDefiActionId(planned.actionId());
            row.setStepsSummary(summarizeSteps(planned));
            actionRepository.save(row);
        } catch (Exception e) {
            log.warn("failed to persist planned action for session={}: {}", session, e.getMessage());
        }
    }

    private static String sessionIdFrom(ToolContext toolContext) {
        if (toolContext != null && toolContext.getContext() != null) {
            Object sid = toolContext.getContext().get("sessionId");
            if (sid != null && !sid.toString().isBlank()) {
                return sid.toString();
            }
        }
        return SessionContext.get();
    }

    private static String summarizeSteps(ExecutionAction action) {
        if (action.steps() == null || action.steps().isEmpty()) {
            return null;
        }
        StringBuilder sb = new StringBuilder();
        for (ActionStep step : action.steps()) {
            if (sb.length() > 0) {
                sb.append("; ");
            }
            sb.append(step.type()).append(":").append(step.status());
        }
        return sb.length() > 2000 ? sb.substring(0, 2000) : sb.toString();
    }

    // ── Action management ────────────────────────────────────────────────

    @Tool(description = "List recent planned/executed actions (history).")
    public List<ExecutionAction> actionsList(@ToolParam(description = "Max results, default 10", required = false) Integer limit) {
        return cli.actionsList(limit == null ? 10 : limit);
    }

    @Tool(description = "Show full details of a specific action by id, including its on-chain step calldata.")
    public ExecutionAction actionShow(@ToolParam(description = "Action id") String actionId) {
        return cli.actionsShow(actionId);
    }
}
