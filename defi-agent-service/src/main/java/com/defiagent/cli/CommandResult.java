package com.defiagent.cli;

/** Raw result of a defi-cli subprocess invocation, before JSON parsing. */
public record CommandResult(int exitCode, String output) {
}
