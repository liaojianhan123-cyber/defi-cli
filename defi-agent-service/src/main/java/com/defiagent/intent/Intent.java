package com.defiagent.intent;

/**
 * Parsed user intent. Kept intentionally small for the demo skeleton.
 *
 * <p>{@code address}, if present, lets {@code AgentService.plan(...)} attempt a
 * real {@code DefiCliClient} plan (which requires a from-address) in addition
 * to the always-on paper plan; it is optional so the flow still works when the
 * user hasn't mentioned a wallet.
 */
public record Intent(
        Type type,
        String asset,
        String chain,
        String amount,
        String address
) {
    public enum Type {
        DEPOSIT,   // deposit / earn yield
        CONFIRM,   // confirm the pending action
        STATUS,    // query status
        HELP       // anything else
    }

    public static Intent help() {
        return new Intent(Type.HELP, null, null, null, null);
    }
}
