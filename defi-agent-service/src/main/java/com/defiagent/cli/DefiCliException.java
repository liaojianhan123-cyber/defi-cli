package com.defiagent.cli;

/**
 * Typed error surfaced from a defi-cli invocation. {@code code} mirrors
 * defi-cli's stable exit-code table (internal/errors/errors.go): 2=usage,
 * 10=auth, 11=rate_limited, 12=unavailable, 13=unsupported, 14=stale,
 * 16=blocked, 20-24=action/execution errors. Callers (e.g. the REST
 * exception handler) can map these to HTTP status codes without depending on
 * Go internals.
 */
public class DefiCliException extends RuntimeException {

    private final int code;
    private final String type;

    public DefiCliException(int code, String type, String message) {
        super(message);
        this.code = code;
        this.type = type;
    }

    public DefiCliException(int code, String type, String message, Throwable cause) {
        super(message, cause);
        this.code = code;
        this.type = type;
    }

    public int getCode() {
        return code;
    }

    public String getType() {
        return type;
    }

    /** For failures on the Java side (process start/timeout/parse), not defi-cli's own error envelope. */
    public static DefiCliException internal(String message, Throwable cause) {
        return new DefiCliException(1, "internal", message, cause);
    }
}
