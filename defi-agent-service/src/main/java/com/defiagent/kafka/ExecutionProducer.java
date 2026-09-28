package com.defiagent.kafka;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.kafka.core.KafkaTemplate;
import org.springframework.stereotype.Component;

/**
 * Publishes an execution command (the actionId) to Kafka.
 * Decouples the synchronous /chat confirm from the async broadcast.
 */
@Component
public class ExecutionProducer {

    private static final Logger log = LoggerFactory.getLogger(ExecutionProducer.class);

    private final KafkaTemplate<String, String> kafkaTemplate;
    private final String topic;

    public ExecutionProducer(KafkaTemplate<String, String> kafkaTemplate,
                             @Value("${app.execution.topic}") String topic) {
        this.kafkaTemplate = kafkaTemplate;
        this.topic = topic;
    }

    public void publish(String actionId) {
        log.info("publishing execution command actionId={} topic={}", actionId, topic);
        kafkaTemplate.send(topic, actionId, actionId);
    }
}
