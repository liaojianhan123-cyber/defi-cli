package com.defiagent.cli;

import java.time.Duration;
import java.util.List;

/**
 * Abstraction over "run a command and capture its output", so
 * {@link DefiCliClient} can be unit-tested with a fake runner instead of
 * depending on a real {@code defi} binary.
 */
public interface CommandRunner {

    /**
     * Runs a command (without the binary path / {@code --results-only} prefix,
     * which the implementation adds) and returns its combined stdout+stderr
     * output and exit code.
     *
     * @throws DefiCliException if the process cannot be started, times out, or
     *                           its output cannot be read
     */
    CommandResult run(List<String> args, Duration timeout);
}
