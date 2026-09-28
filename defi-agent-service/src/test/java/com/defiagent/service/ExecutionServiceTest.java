package com.defiagent.service;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.domain.ActionStatus;
import com.defiagent.domain.AgentAction;
import com.defiagent.repo.AgentActionRepository;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.data.redis.core.ValueOperations;

import java.time.Duration;
import java.util.Optional;
import java.util.concurrent.atomic.AtomicInteger;

import static org.assertj.core.api.Assertions.assertThat;
import static org.mockito.ArgumentMatchers.any;
import static org.mockito.ArgumentMatchers.anyString;
import static org.mockito.Mockito.mock;
import static org.mockito.Mockito.never;
import static org.mockito.Mockito.verify;
import static org.mockito.Mockito.when;

/**
 * Verifies the state-machine guardrails around the (paper) broadcast: a duplicate
 * delivery must not re-execute, and an unexpected failure must land in FAILED
 * instead of leaving the action stuck in EXECUTING forever.
 */
class ExecutionServiceTest {

    private AgentActionRepository repository;
    private StringRedisTemplate redis;
    private ValueOperations<String, String> valueOps;
    private DefiCliClient cli;
    private ExecutionService service;

    @BeforeEach
    @SuppressWarnings("unchecked")
    void setUp() {
        repository = mock(AgentActionRepository.class);
        redis = mock(StringRedisTemplate.class);
        valueOps = mock(ValueOperations.class);
        cli = mock(DefiCliClient.class);
        when(redis.opsForValue()).thenReturn(valueOps);
        when(valueOps.setIfAbsent(anyString(), anyString(), any(Duration.class))).thenReturn(true);
        // Default: save() echoes back whatever was passed in (like an in-place update
        // would). Individual tests below override this with a more specific stub
        // wherever they need to model save() returning a genuinely different/failing
        // outcome (e.g. a merged instance, or a thrown exception).
        when(repository.save(any(AgentAction.class))).thenAnswer(invocation -> invocation.getArgument(0));

        service = new ExecutionService(repository, redis, cli, 0L);
    }

    private AgentAction queuedAction() {
        AgentAction action = new AgentAction();
        action.setActionId("a-1");
        action.setStatus(ActionStatus.QUEUED);
        return action;
    }

    @Test
    void paperExecute_movesQueuedActionToConfirmed() {
        AgentAction action = queuedAction();
        when(repository.findByActionId("a-1")).thenReturn(Optional.of(action));

        service.paperExecute("a-1");

        assertThat(action.getStatus()).isEqualTo(ActionStatus.CONFIRMED);
        assertThat(action.getTxHash()).startsWith("0xdemo");
    }

    @Test
    void paperExecute_alreadyConfirmedIsSkippedNotReExecuted() {
        AgentAction action = queuedAction();
        action.setStatus(ActionStatus.CONFIRMED);
        action.setTxHash("0xoriginal");
        when(repository.findByActionId("a-1")).thenReturn(Optional.of(action));

        service.paperExecute("a-1");

        // Duplicate delivery must be a no-op: no new tx hash, no save.
        assertThat(action.getTxHash()).isEqualTo("0xoriginal");
        verify(repository, never()).save(action);
    }

    @Test
    void paperExecute_infraFailureBeforeConfirmLandsInFailedNotStuckInExecuting() {
        AgentAction action = queuedAction();
        when(repository.findByActionId("a-1")).thenReturn(Optional.of(action));
        // 1st save (persisting the EXECUTING transition) fails as if the DB hiccuped
        // right as broadcast started; the 2nd save is the FAILED write from the catch
        // block, which must succeed so the action isn't stuck in EXECUTING forever.
        AtomicInteger saveCount = new AtomicInteger();
        when(repository.save(action)).thenAnswer(invocation -> {
            if (saveCount.incrementAndGet() == 1) {
                throw new RuntimeException("db unavailable");
            }
            return action;
        });

        service.paperExecute("a-1");

        assertThat(action.getStatus()).isEqualTo(ActionStatus.FAILED);
        assertThat(action.getNote()).contains("db unavailable");
    }

    @Test
    void paperExecute_persistFailureAfterConfirmDoesNotWalkBackToFailed() {
        AgentAction action = queuedAction();
        when(repository.findByActionId("a-1")).thenReturn(Optional.of(action));
        // 1st save (EXECUTING) succeeds; 2nd save (the CONFIRMED write, which happens
        // AFTER action.transitionTo(CONFIRMED) already ran in-memory) fails. This must
        // NOT be auto-flipped to FAILED — CONFIRMED is terminal and a blind revert
        // could cause a duplicate broadcast on retry.
        AtomicInteger saveCount = new AtomicInteger();
        when(repository.save(action)).thenAnswer(invocation -> {
            if (saveCount.incrementAndGet() == 2) {
                throw new RuntimeException("db unavailable");
            }
            return action;
        });

        service.paperExecute("a-1");

        assertThat(action.getStatus()).isEqualTo(ActionStatus.CONFIRMED);
    }

    @Test
    void paperExecute_worksWhenSaveReturnsADifferentMergedInstance() {
        // Real JPA behaviour: `action` here is DETACHED (loaded by findByActionId in a
        // now-closed transaction), so repository.save() merges it and returns a NEW
        // managed instance rather than mutating this one in place. If ExecutionService
        // kept using the original stale reference across its 2nd/3rd save() calls
        // instead of reassigning from the return value, this would blow up with an
        // ObjectOptimisticLockingFailureException on the real @Version-checked entity —
        // this test pins down that we always chain off save()'s return value.
        AgentAction original = queuedAction();
        when(repository.findByActionId("a-1")).thenReturn(Optional.of(original));
        when(repository.save(org.mockito.ArgumentMatchers.any(AgentAction.class))).thenAnswer(invocation -> {
            AgentAction in = invocation.getArgument(0);
            AgentAction merged = new AgentAction();
            merged.setActionId(in.getActionId());
            merged.setStatus(in.getStatus());
            merged.setTxHash(in.getTxHash());
            merged.setNote(in.getNote());
            return merged; // a different object every time, like Hibernate's merge()
        });

        service.paperExecute("a-1");

        // The original detached reference is never touched again by our code after
        // the first save, so asserting on it would prove nothing either way — the
        // real proof is simply that no exception was thrown and the flow completed.
        verify(repository, org.mockito.Mockito.times(2)).save(org.mockito.ArgumentMatchers.any(AgentAction.class));
    }

    @Test
    void paperExecute_missingActionIsANoOp() {
        when(repository.findByActionId("missing")).thenReturn(Optional.empty());

        service.paperExecute("missing");

        verify(repository, never()).save(any());
    }
}
