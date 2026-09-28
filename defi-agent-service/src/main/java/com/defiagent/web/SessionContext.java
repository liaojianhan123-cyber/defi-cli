package com.defiagent.web;

/**
 * Request-scoped conversation id so tool callbacks (which have no HTTP
 * signature) can persist a planned {@code AgentAction} against the same session
 * the chat turn belongs to.
 */
public final class SessionContext {

    private static final ThreadLocal<String> SESSION = new ThreadLocal<>();

    private SessionContext() {
    }

    public static void set(String sessionId) {
        SESSION.set(sessionId);
    }

    public static String get() {
        return SESSION.get();
    }

    public static void clear() {
        SESSION.remove();
    }
}
