package com.defiagent.domain;

import java.util.EnumMap;
import java.util.EnumSet;
import java.util.Map;
import java.util.Set;

/**
 * Lifecycle of an agent action, and the only legal transitions between states.
 *
 * <pre>
 *   PLANNED --confirm--> QUEUED --consumer picks up--> EXECUTING --broadcast ok--> CONFIRMED
 *      |                    |                              |
 *      +--cancel--> CANCELLED                          FAILED <---broadcast/estimate error
 *                                                           |
 *                                                           +--manual retry--> QUEUED
 * </pre>
 *
 * <p>CONFIRMED and CANCELLED are terminal. Every other transition not listed in
 * {@link #TRANSITIONS} is illegal and must be rejected by {@link AgentAction#transitionTo}
 * so a duplicate Kafka delivery or a racing request can never silently corrupt state
 * (e.g. re-broadcasting an already-CONFIRMED action).
 */
public enum ActionStatus {
    PLANNED,
    QUEUED,
    EXECUTING,
    CONFIRMED,
    FAILED,
    CANCELLED;

    private static final Map<ActionStatus, Set<ActionStatus>> TRANSITIONS = new EnumMap<>(ActionStatus.class);

    static {
        TRANSITIONS.put(PLANNED, EnumSet.of(QUEUED, CANCELLED));
        TRANSITIONS.put(QUEUED, EnumSet.of(EXECUTING, CANCELLED, FAILED));
        TRANSITIONS.put(EXECUTING, EnumSet.of(CONFIRMED, FAILED));
        TRANSITIONS.put(FAILED, EnumSet.of(QUEUED));
        TRANSITIONS.put(CONFIRMED, EnumSet.noneOf(ActionStatus.class));
        TRANSITIONS.put(CANCELLED, EnumSet.noneOf(ActionStatus.class));
    }

    public boolean canTransitionTo(ActionStatus target) {
        return TRANSITIONS.getOrDefault(this, Set.of()).contains(target);
    }

    public boolean isTerminal() {
        return this == CONFIRMED || this == CANCELLED;
    }
}
