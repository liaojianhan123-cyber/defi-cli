package com.defiagent.kafka;

import com.defiagent.service.ExecutionService;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.kafka.annotation.KafkaListener;
import org.springframework.stereotype.Component;

/**
 * Consumes execution commands and performs the (paper) broadcast asynchronously.
 */
@Component
public class ExecutionConsumer {

    private static final Logger log = LoggerFactory.getLogger(ExecutionConsumer.class);

    private final ExecutionService executionService;

    public ExecutionConsumer(ExecutionService executionService) {
        this.executionService = executionService;
    }

    @KafkaListener(topics = "${app.execution.topic}", groupId = "defi-agent")
    public void onMessage(String actionId) {
        log.info("received execution command actionId={}", actionId);
        try {
            executionService.paperExecute(actionId);
        } catch (Exception e) {
            log.error("execution failed actionId={} err={}", actionId, e.getMessage(), e);
        }
    }
}
