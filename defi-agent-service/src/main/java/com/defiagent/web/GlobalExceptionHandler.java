package com.defiagent.web;

import com.defiagent.cli.DefiCliException;
import com.defiagent.domain.IllegalActionStateException;
import com.defiagent.web.dto.ApiError;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.ExceptionHandler;
import org.springframework.web.bind.annotation.RestControllerAdvice;
import org.springframework.web.server.ResponseStatusException;

/**
 * Maps {@link DefiCliException} codes to HTTP status using the same table as
 * defi-cli's own exit codes (internal/errors/errors.go), so a Java client can
 * reason about failures with the same semantics as the CLI.
 */
@RestControllerAdvice
public class GlobalExceptionHandler {

    private static final Logger log = LoggerFactory.getLogger(GlobalExceptionHandler.class);

    @ExceptionHandler(DefiCliException.class)
    public ResponseEntity<ApiError> handleDefiCliException(DefiCliException ex) {
        log.warn("defi-cli error code={} type={} message={}", ex.getCode(), ex.getType(), ex.getMessage());
        HttpStatus status = mapStatus(ex.getCode());
        return ResponseEntity.status(status).body(new ApiError(ex.getCode(), ex.getType(), ex.getMessage()));
    }

    @ExceptionHandler(IllegalActionStateException.class)
    public ResponseEntity<ApiError> handleIllegalActionState(IllegalActionStateException ex) {
        log.warn("illegal action state transition: {}", ex.getMessage());
        return ResponseEntity.status(HttpStatus.CONFLICT)
                .body(new ApiError(HttpStatus.CONFLICT.value(), "illegal_state_transition", ex.getMessage()));
    }

    @ExceptionHandler(ResponseStatusException.class)
    public ResponseEntity<ApiError> handleResponseStatusException(ResponseStatusException ex) {
        return ResponseEntity.status(ex.getStatusCode())
                .body(new ApiError(ex.getStatusCode().value(), "request_error", ex.getReason()));
    }

    private HttpStatus mapStatus(int code) {
        return switch (code) {
            case 2 -> HttpStatus.BAD_REQUEST;           // usage
            case 10 -> HttpStatus.UNAUTHORIZED;         // auth (missing provider API key)
            case 11 -> HttpStatus.TOO_MANY_REQUESTS;    // rate_limited
            case 12 -> HttpStatus.SERVICE_UNAVAILABLE;  // unavailable
            case 13 -> HttpStatus.NOT_IMPLEMENTED;      // unsupported
            case 14 -> HttpStatus.CONFLICT;             // stale
            case 15 -> HttpStatus.CONFLICT;             // partial_strict
            case 16, 22 -> HttpStatus.FORBIDDEN;        // blocked / action_policy
            case 20, 21, 23, 24 -> HttpStatus.UNPROCESSABLE_ENTITY; // action plan/sim/timeout/signer
            default -> HttpStatus.INTERNAL_SERVER_ERROR;
        };
    }
}
