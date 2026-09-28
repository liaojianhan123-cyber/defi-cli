package com.defiagent.cli.model;

import com.fasterxml.jackson.databind.JsonNode;

import java.util.List;

/**
 * Mirrors defi-cli's stable JSON envelope (internal/model/types.go Envelope).
 * {@code data} is kept as a raw {@link JsonNode} here and converted to a concrete
 * type only after we know whether the caller expects a single object or a list.
 */
public record Envelope(
        String version,
        boolean success,
        JsonNode data,
        ErrorBody error,
        List<String> warnings
) {
}
