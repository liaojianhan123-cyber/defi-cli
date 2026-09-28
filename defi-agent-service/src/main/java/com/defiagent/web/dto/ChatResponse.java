package com.defiagent.web.dto;

/**
 * Agent reply.
 *
 * @param reply     human-readable assistant message
 * @param actionId  the action this turn relates to (nullable)
 * @param status    action status (nullable)
 * @param data      optional structured payload (e.g. ranked opportunities)
 */
public record ChatResponse(String reply, String actionId, String status, Object data) {

    public static ChatResponse text(String reply) {
        return new ChatResponse(reply, null, null, null);
    }
}
