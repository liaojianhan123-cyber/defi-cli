package com.defiagent.domain;

/**
 * Thrown when code attempts an {@link ActionStatus} transition that
 * {@link ActionStatus#canTransitionTo} does not allow, e.g. a duplicate Kafka
 * delivery trying to move an already-CONFIRMED action back to EXECUTING.
 *
 * <p>Callers on the async path (Kafka consumer) should catch this, log it and
 * skip — it usually signals a harmless duplicate/race, not a real failure.
 * Callers on the sync path (REST) should let it surface as HTTP 409 (see
 * {@code GlobalExceptionHandler}) so the client knows the action already moved on.
 */
public class IllegalActionStateException extends RuntimeException {

    private final String actionId;
    private final ActionStatus from;
    private final ActionStatus to;

    public IllegalActionStateException(String actionId, ActionStatus from, ActionStatus to) {
        super("action " + actionId + " cannot transition from " + from + " to " + to);
        this.actionId = actionId;
        this.from = from;
        this.to = to;
    }

    public String getActionId() {
        return actionId;
    }

    public ActionStatus getFrom() {
        return from;
    }

    public ActionStatus getTo() {
        return to;
    }
}
