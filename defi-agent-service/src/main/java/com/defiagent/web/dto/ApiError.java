package com.defiagent.web.dto;

/** Structured error body, mirroring defi-cli's own {@code {code,type,message}} error contract. */
public record ApiError(int code, String type, String message) {
}
