package com.defiagent.domain;

import org.junit.jupiter.api.Test;

import static org.assertj.core.api.Assertions.assertThat;
import static org.assertj.core.api.Assertions.assertThatThrownBy;

/** The state machine is the safety net for the async execution chain: every transition here is load-bearing. */
class AgentActionTest {

    private AgentAction actionWith(ActionStatus status) {
        AgentAction action = new AgentAction();
        action.setActionId("test-action-1");
        action.setStatus(status);
        return action;
    }

    @Test
    void happyPath_plannedToQueuedToExecutingToConfirmed() {
        AgentAction action = actionWith(ActionStatus.PLANNED);

        action.transitionTo(ActionStatus.QUEUED);
        assertThat(action.getStatus()).isEqualTo(ActionStatus.QUEUED);

        action.transitionTo(ActionStatus.EXECUTING);
        assertThat(action.getStatus()).isEqualTo(ActionStatus.EXECUTING);

        action.transitionTo(ActionStatus.CONFIRMED);
        assertThat(action.getStatus()).isEqualTo(ActionStatus.CONFIRMED);
    }

    @Test
    void confirmedIsTerminal_rejectsAnyFurtherTransition() {
        AgentAction action = actionWith(ActionStatus.CONFIRMED);

        assertThatThrownBy(() -> action.transitionTo(ActionStatus.EXECUTING))
                .isInstanceOf(IllegalActionStateException.class)
                .hasMessageContaining("test-action-1")
                .hasMessageContaining("CONFIRMED")
                .hasMessageContaining("EXECUTING");

        // The rejected call must not have mutated state.
        assertThat(action.getStatus()).isEqualTo(ActionStatus.CONFIRMED);
    }

    @Test
    void duplicateKafkaDelivery_cannotReenterExecutingFromExecuting() {
        AgentAction action = actionWith(ActionStatus.EXECUTING);

        assertThatThrownBy(() -> action.transitionTo(ActionStatus.EXECUTING))
                .isInstanceOf(IllegalActionStateException.class);
    }

    @Test
    void failedActionCanBeRetriedByRequeuing() {
        AgentAction action = actionWith(ActionStatus.FAILED);

        action.transitionTo(ActionStatus.QUEUED);

        assertThat(action.getStatus()).isEqualTo(ActionStatus.QUEUED);
    }

    @Test
    void cancelledIsTerminal() {
        AgentAction action = actionWith(ActionStatus.CANCELLED);

        assertThatThrownBy(() -> action.transitionTo(ActionStatus.QUEUED))
                .isInstanceOf(IllegalActionStateException.class);
    }

    @Test
    void plannedCanBeCancelledButNotExecutedDirectly() {
        AgentAction action = actionWith(ActionStatus.PLANNED);

        assertThatThrownBy(() -> action.transitionTo(ActionStatus.EXECUTING))
                .isInstanceOf(IllegalActionStateException.class);

        action.transitionTo(ActionStatus.CANCELLED);
        assertThat(action.getStatus()).isEqualTo(ActionStatus.CANCELLED);
    }
}
