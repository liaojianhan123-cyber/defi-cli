package com.defiagent.guardrails;

/** Result of a blocking guardrail check. {@code reason} is null when {@code allowed} is true. */
public record GuardrailResult(boolean allowed, String reason) {

    public static GuardrailResult allow() {
        return new GuardrailResult(true, null);
    }

    public static GuardrailResult blocked(String reason) {
        return new GuardrailResult(false, reason);
    }
}
