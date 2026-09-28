package com.defiagent.cli;

import org.springframework.boot.context.properties.ConfigurationProperties;

import java.time.Duration;

/**
 * Binds {@code defi.cli.*} configuration (path to the Go binary, default
 * per-command timeout). Override via env var {@code DEFI_CLI_PATH} or
 * {@code defi.cli.path} in application.yml.
 */
@ConfigurationProperties(prefix = "defi.cli")
public class DefiCliProperties {

    /** Path to the defi-cli binary; defaults to "defi" resolved via PATH. */
    private String path = "defi";

    /** Default timeout applied when a command-specific timeout is not given. */
    private Duration timeout = Duration.ofSeconds(10);

    public String getPath() {
        return path;
    }

    public void setPath(String path) {
        this.path = path;
    }

    public Duration getTimeout() {
        return timeout;
    }

    public void setTimeout(Duration timeout) {
        this.timeout = timeout;
    }
}
