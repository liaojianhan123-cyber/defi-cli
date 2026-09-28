package com.defiagent.cli.model;

/** Mirrors defi-cli's execution.StepCall (one EVM call inside a batched step). */
public record StepCall(String target, String data, String value) {
}
