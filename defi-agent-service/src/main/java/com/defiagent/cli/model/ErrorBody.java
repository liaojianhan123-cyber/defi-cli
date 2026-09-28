package com.defiagent.cli.model;

/** Mirrors defi-cli's ErrorBody (internal/model/types.go). */
public record ErrorBody(int code, String type, String message) {
}
