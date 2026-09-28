package com.defiagent.cli;

import jakarta.annotation.PreDestroy;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.stereotype.Component;

import java.io.IOException;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;
import java.time.Duration;
import java.util.ArrayList;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;
import java.util.concurrent.TimeoutException;

/**
 * Real {@link CommandRunner}: spawns {@code defi --results-only <args...>} as
 * a subprocess and captures its combined stdout+stderr.
 *
 * <p>Output is read on a background thread so a hung/slow process can be
 * killed on timeout without risking a pipe-buffer deadlock (the reader thread
 * keeps draining the pipe while the calling thread waits with a bound).
 */
@Component
public class DefiCliCommandRunner implements CommandRunner {

    private static final Logger log = LoggerFactory.getLogger(DefiCliCommandRunner.class);

    private final DefiCliProperties properties;
    private final ExecutorService ioExecutor = Executors.newCachedThreadPool(r -> {
        Thread t = new Thread(r, "defi-cli-io");
        t.setDaemon(true);
        return t;
    });

    public DefiCliCommandRunner(DefiCliProperties properties) {
        this.properties = properties;
    }

    @Override
    public CommandResult run(List<String> args, Duration timeout) {
        Duration effectiveTimeout = timeout != null ? timeout : properties.getTimeout();

        List<String> command = new ArrayList<>(args.size() + 2);
        command.add(properties.getPath());
        command.add("--results-only");
        command.addAll(args);

        log.debug("running defi-cli command: {}", command);

        Process process;
        try {
            process = new ProcessBuilder(command).redirectErrorStream(true).start();
        } catch (IOException e) {
            throw DefiCliException.internal(
                    "defi binary not found or not executable at '" + properties.getPath()
                            + "'. Build it with: cd defi-cli && go build -o defi ./cmd/defi (or set DEFI_CLI_PATH)",
                    e);
        }

        Future<byte[]> outputFuture = ioExecutor.submit(() -> readAll(process.getInputStream()));
        try {
            byte[] bytes = outputFuture.get(effectiveTimeout.toMillis(), TimeUnit.MILLISECONDS);
            boolean exited = process.waitFor(2, TimeUnit.SECONDS);
            int exitCode = exited ? process.exitValue() : -1;
            return new CommandResult(exitCode, new String(bytes, StandardCharsets.UTF_8).trim());
        } catch (TimeoutException e) {
            outputFuture.cancel(true);
            process.destroyForcibly();
            throw DefiCliException.internal("defi-cli command timed out after " + effectiveTimeout, e);
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            process.destroyForcibly();
            throw DefiCliException.internal("defi-cli command execution interrupted", e);
        } catch (Exception e) {
            process.destroyForcibly();
            throw DefiCliException.internal("defi-cli command execution failed", e);
        }
    }

    private byte[] readAll(InputStream in) throws IOException {
        return in.readAllBytes();
    }

    @PreDestroy
    void shutdown() {
        ioExecutor.shutdownNow();
    }
}
