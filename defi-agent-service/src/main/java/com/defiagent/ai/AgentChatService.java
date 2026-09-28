package com.defiagent.ai;

import com.defiagent.guardrails.AgentProperties;
import org.springframework.ai.chat.client.ChatClient;
import org.springframework.ai.chat.client.advisor.MessageChatMemoryAdvisor;
import org.springframework.ai.chat.memory.ChatMemory;
import org.springframework.stereotype.Service;
import reactor.core.publisher.Flux;

/**
 * LLM-driven orchestration layer: translates a natural-language message into
 * structured {@link DefiToolService} tool calls via Spring AI's ChatClient
 * tool-calling loop, one conversation per {@code sessionId} (Spring AI
 * {@link ChatMemory}, auto-configured with an in-memory sliding window).
 */
@Service
public class AgentChatService {

    private final ChatClient chatClient;

    public AgentChatService(ChatClient.Builder chatClientBuilder,
                             DefiToolService defiToolService,
                             ChatMemory chatMemory,
                             AgentProperties agentProperties) {
        this.chatClient = chatClientBuilder
                .defaultSystem(SystemPrompts.build(agentProperties))
                .defaultAdvisors(MessageChatMemoryAdvisor.builder(chatMemory).build())
                .defaultTools(defiToolService)
                .build();
    }

    public String chat(String sessionId, String message) {
        return chatClient.prompt()
                .user(message)
                .advisors(a -> a.param(ChatMemory.CONVERSATION_ID, sessionId))
                .toolContext(java.util.Map.of("sessionId", sessionId))
                .call()
                .content();
    }

    /** Token stream for the UI. Tool-calling still happens inside this loop when the model needs it. */
    public Flux<String> stream(String sessionId, String message) {
        return chatClient.prompt()
                .user(message)
                .advisors(a -> a.param(ChatMemory.CONVERSATION_ID, sessionId))
                .toolContext(java.util.Map.of("sessionId", sessionId))
                .stream()
                .content();
    }
}
