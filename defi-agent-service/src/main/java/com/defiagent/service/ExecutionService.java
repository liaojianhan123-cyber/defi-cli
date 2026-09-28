package com.defiagent.service;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.cli.DefiCliException;
import com.defiagent.cli.model.ActionGasEstimate;
import com.defiagent.cli.model.ActionGasEstimateChainTotal;
import com.defiagent.domain.ActionStatus;
import com.defiagent.domain.AgentAction;
import com.defiagent.domain.IllegalActionStateException;
import com.defiagent.repo.AgentActionRepository;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.util.UUID;

/**
 * Performs the "execution" of a confirmed action.
 *
 * <p><b>Paper mode</b>: no real wallet, no real broadcast. We simulate latency and
 * return a deterministic demo tx hash. The persistence + idempotency + status
 * machinery is real, which is what the JD tech stack is about.
 *
 * <p>When the action carries a real {@code defiActionId} (see {@code AgentService}),
 * this also fetches a REAL gas/fee estimate from defi-cli right before the
 * simulated broadcast — plan and estimate are real, only the final broadcast is
 * paper. This boundary is deliberate: see README "Architecture & Boundaries".
 */
@Service
public class ExecutionService {

    private static final Logger log = LoggerFactory.getLogger(ExecutionService.class);

    private final AgentActionRepository repository;
    private final StringRedisTemplate redis;
    private final DefiCliClient cli;
    private final long simulatedLatencyMs;

    public ExecutionService(AgentActionRepository repository,
                            StringRedisTemplate redis,
                            DefiCliClient cli,
                            @Value("${app.execution.simulated-latency-ms:1000}") long simulatedLatencyMs) {
        this.repository = repository;
        this.redis = redis;
        this.cli = cli;
        this.simulatedLatencyMs = simulatedLatencyMs;
    }

    public void paperExecute(String actionId) {
        // Idempotency: a duplicate Kafka delivery must not double-execute.
        String idemKey = "executed:" + actionId;
        Boolean firstTime = redis.opsForValue().setIfAbsent(idemKey, "1", Duration.ofHours(1));
        if (Boolean.FALSE.equals(firstTime)) {
            log.warn("duplicate execution ignored actionId={}", actionId);
            return;
        }

        AgentAction action = repository.findByActionId(actionId).orElse(null);
        if (action == null) {
            log.warn("action not found actionId={}", actionId);
            return;
        }

        try {
            action.transitionTo(ActionStatus.EXECUTING);
        } catch (IllegalActionStateException e) {
            // Already EXECUTING/CONFIRMED/FAILED/CANCELLED: a duplicate Kafka delivery
            // or a racing consumer got here first. Nothing to do — this is the
            // legitimate "harmless duplicate" case, not an error.
            log.info("skip execution, action not QUEUED: {}", e.getMessage());
            return;
        }

        try {
            // IMPORTANT: reassign from save()'s return value everywhere below. `action`
            // was loaded via findByActionId in a prior (now-closed) repository-method
            // transaction, so it's a DETACHED entity; save() on a detached entity with
            // a non-null id merges it and hands back a NEW managed instance with the
            // bumped @Version. If we kept using the old `action` reference instead, its
            // stale version would fail the optimistic-lock check on the *next* save with
            // "Row was updated or deleted by another transaction" — self-inflicted, not
            // a real concurrent writer. This bit us on the very first real run.
            action = repository.save(action);

            String estimateNote = fetchRealEstimateNote(action);

            // Simulate broadcast latency so the async transition is observable.
            try {
                Thread.sleep(simulatedLatencyMs);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                throw e;
            }

            String txHash = "0xdemo" + UUID.randomUUID().toString().replace("-", "");
            action.setTxHash(txHash);
            action.transitionTo(ActionStatus.CONFIRMED);
            String note = "paper execution — real plan" + (estimateNote != null ? " + real estimate" : "")
                    + ", simulated broadcast (no signing/no real tx)"
                    + (estimateNote != null ? "; " + estimateNote : "");
            action.setNote(note.length() > 480 ? note.substring(0, 480) : note);
            action = repository.save(action);

            log.info("paper execution confirmed actionId={} txHash={}", actionId, txHash);
        } catch (Exception e) {
            log.error("paper execution failed actionId={} err={}", actionId, e.getMessage(), e);
            if (action.getStatus().isTerminal()) {
                // The in-memory transitionTo(CONFIRMED) already happened before this
                // failure hit (e.g. the final persist itself is what failed). We can't
                // safely walk CONFIRMED back to FAILED: a naive retry after that could
                // double-broadcast a transaction that may have already gone through.
                // Surface loudly for manual reconciliation instead of guessing.
                log.error("actionId={} reached CONFIRMED in-memory but persistence failed after; "
                        + "needs manual reconciliation, NOT auto-marking FAILED", actionId);
                return;
            }
            action.transitionTo(ActionStatus.FAILED);
            String note = "execution failed: " + e.getMessage();
            action.setNote(note.length() > 480 ? note.substring(0, 480) : note);
            repository.save(action);
        }
    }

    /** Best-effort real gas/fee estimate; never blocks the paper broadcast on failure. */
    private String fetchRealEstimateNote(AgentAction action) {
        if (action.getDefiActionId() == null || action.getDefiActionId().isBlank()) {
            return null;
        }
        try {
            ActionGasEstimate estimate = cli.estimate(action.getDefiActionId());
            if (estimate == null || estimate.totalsByChain() == null || estimate.totalsByChain().isEmpty()) {
                return null;
            }
            StringBuilder sb = new StringBuilder("estimated fee:");
            for (ActionGasEstimateChainTotal total : estimate.totalsByChain()) {
                sb.append(' ').append(total.chainId()).append('=').append(total.likelyFeeWei())
                        .append(total.feeUnit() != null && !total.feeUnit().isBlank() ? " " + total.feeUnit() : " wei");
            }
            log.info("real gas estimate fetched for actionId={} defiActionId={}", action.getActionId(), action.getDefiActionId());
            return sb.toString();
        } catch (DefiCliException e) {
            log.info("real gas estimate unavailable (code={} type={}): {}", e.getCode(), e.getType(), e.getMessage());
            return null;
        } catch (Exception e) {
            log.warn("real gas estimate failed unexpectedly: {}", e.getMessage());
            return null;
        }
    }
}
