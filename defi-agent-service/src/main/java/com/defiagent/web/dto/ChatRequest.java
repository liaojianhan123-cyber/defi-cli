package com.defiagent.web.dto;

/**
 * @param sessionId conversation id (any stable string per user/session)
 * @param message   natural language message
 */
public record ChatRequest(String sessionId, String message) {
}
