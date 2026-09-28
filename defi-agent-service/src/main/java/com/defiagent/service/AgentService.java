package com.defiagent.service;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.cli.DefiCliException;
import com.defiagent.cli.model.ActionStep;
import com.defiagent.cli.model.ExecutionAction;
import com.defiagent.domain.ActionStatus;
import com.defiagent.domain.AgentAction;
import com.defiagent.domain.IllegalActionStateException;
import com.defiagent.guardrails.GuardrailResult;
import com.defiagent.guardrails.GuardrailService;
import com.defiagent.intent.Intent;
import com.defiagent.intent.IntentParser;
import com.defiagent.kafka.ExecutionProducer;
import com.defiagent.repo.AgentActionRepository;
import com.defiagent.web.dto.ChatResponse;
import com.defiagent.yield.YieldOpportunity;
import com.defiagent.yield.YieldService;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Service;

import java.util.List;
import java.util.UUID;

/**
 * Orchestrates a single chat turn:
 * parse intent -> (plan | confirm | status | help).
 *
 * <p>This is the rule-based fallback agent used when {@code OPENAI_API_KEY} is
 * not set (see {@code ChatController}); the Spring AI path goes through
 * {@code AgentChatService}/{@code DefiToolService} instead. Both paths funnel
 * into the same {@link AgentAction} state machine and {@link ExecutionService}.
 */
@Service
public class AgentService {

    private static final Logger log = LoggerFactory.getLogger(AgentService.class);

    private final IntentParser intentParser;
    private final YieldService yieldService;
    private final AgentActionRepository repository;
    private final ExecutionProducer executionProducer;
    private final DefiCliClient cli;
    private final GuardrailService guardrails;

    public AgentService(IntentParser intentParser,
                        YieldService yieldService,
                        AgentActionRepository repository,
                        ExecutionProducer executionProducer,
                        DefiCliClient cli,
                        GuardrailService guardrails) {
        this.intentParser = intentParser;
        this.yieldService = yieldService;
        this.repository = repository;
        this.executionProducer = executionProducer;
        this.cli = cli;
        this.guardrails = guardrails;
    }

    public ChatResponse handle(String sessionId, String message) {
        String session = (sessionId == null || sessionId.isBlank()) ? "default" : sessionId;
        Intent intent = intentParser.parse(message);
        log.info("session={} intent={} message={}", session, intent.type(), message);

        return switch (intent.type()) {
            case DEPOSIT -> plan(session, intent);
            case CONFIRM -> confirm(session);
            case STATUS -> status(session);
            case HELP -> ChatResponse.text(helpText());
        };
    }

    private ChatResponse plan(String session, Intent intent) {
        List<YieldOpportunity> top = yieldService.topOpportunities(intent.asset(), intent.chain(), 3);
        if (top.isEmpty()) {
            return ChatResponse.text("没有找到 " + intent.asset() + " 在 " + intent.chain() + " 上的收益机会。");
        }
        YieldOpportunity best = top.get(0);

        AgentAction action = new AgentAction();
        action.setActionId(UUID.randomUUID().toString());
        action.setSessionId(session);
        action.setIntent("DEPOSIT");
        action.setProvider(best.provider());
        action.setChain(best.chain());
        action.setAsset(best.asset());
        action.setAmount(intent.amount());
        action.setApy(best.apy());
        action.setTvlUsd(best.tvlUsd());
        action.setStatus(ActionStatus.PLANNED);

        StringBuilder sb = new StringBuilder();
        sb.append("为你比较了 ").append(top.size()).append(" 个协议，推荐 ")
                .append(best.provider()).append("（").append(best.chain()).append("）：\n");
        for (YieldOpportunity o : top) {
            sb.append(String.format("  - %-10s APY %.2f%%  TVL $%,.0f%n", o.provider(), o.apy(), o.tvlUsd()));
        }
        sb.append(String.format("计划：存入 %s %s 到 %s。%n", intent.amount(), best.asset(), best.provider()));

        // Best-effort: if the user gave a wallet address, also create a REAL defi-cli
        // plan (real action_id + on-chain call steps) so /api/actions/{id}/estimate has
        // something real to estimate. Never blocks the paper plan if this fails.
        if (intent.address() != null) {
            tryCreateRealPlan(action, intent, best, sb);
        }

        repository.save(action);
        sb.append("真上链请在页面点「用钱包执行」（MetaMask 弹窗，私钥不出钱包）。回复 “确认” 仍是 paper 模拟广播。");

        return new ChatResponse(sb.toString(), action.getActionId(), ActionStatus.PLANNED.name(), top);
    }

    private void tryCreateRealPlan(AgentAction action, Intent intent, YieldOpportunity best, StringBuilder sb) {
        GuardrailResult check = guardrails.checkProtocolAllowed(best.provider());
        if (!check.allowed()) {
            log.info("skipping real defi-cli plan, blocked by guardrail: {}", check.reason());
            return;
        }
        try {
            String baseUnits = AssetUnits.toBaseUnits(intent.amount(), best.asset());
            ExecutionAction planned = cli.yieldDepositPlan(
                    best.provider(), best.chain(), best.asset(), baseUnits, intent.address(), null, null);
            if (planned != null && planned.actionId() != null) {
                action.setDefiActionId(planned.actionId());
                action.setStepsSummary(summarizeSteps(planned));
                sb.append("已通过 defi-cli 生成真实 action_id=").append(planned.actionId())
                        .append("（仍未广播，可用 GET /api/actions/").append(action.getActionId())
                        .append("/estimate 查看真实 gas 预估）。\n");
            }
        } catch (DefiCliException e) {
            log.info("real defi-cli plan not created (code={} type={}): {}", e.getCode(), e.getType(), e.getMessage());
        } catch (Exception e) {
            log.warn("real defi-cli plan failed unexpectedly: {}", e.getMessage());
        }
    }

    private String summarizeSteps(ExecutionAction action) {
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

    private ChatResponse confirm(String session) {
        AgentAction pending = repository
                .findFirstBySessionIdAndStatusOrderByCreatedAtDesc(session, ActionStatus.PLANNED)
                .orElse(null);
        if (pending == null) {
            return ChatResponse.text("没有待确认的操作。请先告诉我你想做什么，例如：把 1000 USDC 存到以太坊收益最高的池。");
        }
        try {
            pending.transitionTo(ActionStatus.QUEUED);
        } catch (IllegalActionStateException e) {
            // Racing confirm (e.g. double-click) or the action already moved on; don't
            // publish a second execution command for the same actionId.
            log.warn("confirm rejected: {}", e.getMessage());
            return ChatResponse.text("这个操作已经在处理中或已完成，无需重复确认。回复 “状态” 查询结果。");
        }
        pending = repository.save(pending);
        executionProducer.publish(pending.getActionId());

        return new ChatResponse(
                "已提交 paper 模拟执行（不会弹出 MetaMask、不会动真实资金）。真上链请点页面上的「用钱包执行」。",
                pending.getActionId(),
                ActionStatus.QUEUED.name(),
                null);
    }

    private ChatResponse status(String session) {
        List<AgentAction> recent = repository.findTop20BySessionIdOrderByCreatedAtDesc(session);
        if (recent.isEmpty()) {
            return ChatResponse.text("这个会话还没有任何操作。");
        }
        AgentAction latest = recent.get(0);
        String reply = switch (latest.getStatus()) {
            case CONFIRMED -> "✅ 执行完成，txHash=" + latest.getTxHash();
            case EXECUTING, QUEUED -> "⏳ 正在执行中，请稍候再查。";
            case PLANNED -> "📝 计划已生成，尚未确认。回复 “确认” 执行。";
            case FAILED -> "❌ 执行失败。";
            case CANCELLED -> "已取消。";
        };
        return new ChatResponse(reply, latest.getActionId(), latest.getStatus().name(), latest);
    }

    private String helpText() {
        return """
                我是 DeFi AI Agent。你可以对我说：
                  - 把 1000 USDC 存到以太坊收益最高的池   （生成计划）
                  - 确认                                    （执行，paper 模式）
                  - 状态                                    （查询结果）
                """;
    }
}
