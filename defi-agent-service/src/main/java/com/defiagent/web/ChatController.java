package com.defiagent.web;

import com.defiagent.ai.AgentChatService;
import com.defiagent.domain.ActionStatus;
import com.defiagent.domain.AgentAction;
import com.defiagent.repo.AgentActionRepository;
import com.defiagent.service.AgentService;
import com.defiagent.web.dto.ChatRequest;
import com.defiagent.web.dto.ChatResponse;
import com.defiagent.web.dto.StreamDone;
import com.fasterxml.jackson.databind.ObjectMapper;
import io.swagger.v3.oas.annotations.Operation;
import io.swagger.v3.oas.annotations.tags.Tag;
import jakarta.servlet.http.HttpServletResponse;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.MediaType;
import org.springframework.web.bind.annotation.PostMapping;
import org.springframework.web.bind.annotation.RequestBody;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;
import org.springframework.web.servlet.mvc.method.annotation.SseEmitter;

import java.io.IOException;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ThreadFactory;
import java.util.concurrent.atomic.AtomicInteger;

/**
 * Routes chat turns to the Spring AI + OpenAI tool-calling agent when
 * {@code OPENAI_API_KEY} is configured, otherwise degrades to the original
 * rule-based {@link AgentService} (no external dependency, works offline).
 */
@RestController
@RequestMapping("/api")
@Tag(name = "Agent", description = "Natural-language DeFi agent (Spring AI + OpenAI tool-calling, with a rule-based fallback)")
public class ChatController {

    private static final Logger log = LoggerFactory.getLogger(ChatController.class);

    private final AgentService ruleBasedAgentService;
    private final AgentChatService llmAgentChatService;
    private final AgentActionRepository actionRepository;
    private final ObjectMapper objectMapper;
    private final boolean llmEnabled;
    private final ExecutorService streamExecutor;

    public ChatController(AgentService ruleBasedAgentService,
                           AgentChatService llmAgentChatService,
                           AgentActionRepository actionRepository,
                           ObjectMapper objectMapper,
                           // NOTE: read the raw env var, not spring.ai.openai.api-key — that
                           // property is filled with a non-blank placeholder (see application.yml)
                           // so the OpenAI autoconfiguration doesn't crash the context on startup.
                           @Value("${OPENAI_API_KEY:}") String openAiApiKey) {
        this.ruleBasedAgentService = ruleBasedAgentService;
        this.llmAgentChatService = llmAgentChatService;
        this.actionRepository = actionRepository;
        this.objectMapper = objectMapper;
        this.llmEnabled = openAiApiKey != null && !openAiApiKey.isBlank();
        this.streamExecutor = Executors.newCachedThreadPool(new StreamThreadFactory());
        log.info("chat agent mode: {}", llmEnabled ? "spring-ai (OpenAI tool-calling)" : "rule-based fallback (no OPENAI_API_KEY)");
    }

    @PostMapping("/chat")
    @Operation(summary = "Send a natural-language message to the agent")
    public ChatResponse chat(@RequestBody ChatRequest request) {
        if (llmEnabled) {
            try {
                SessionContext.set(request.sessionId());
                String reply = llmAgentChatService.chat(request.sessionId(), request.message());
                return withPendingAction(request.sessionId(), reply);
            } finally {
                SessionContext.clear();
            }
        }
        return ruleBasedAgentService.handle(request.sessionId(), request.message());
    }

    @PostMapping(value = "/chat/stream", produces = MediaType.TEXT_EVENT_STREAM_VALUE)
    @Operation(summary = "Stream the agent reply as SSE token deltas (same orchestration as /chat)")
    public SseEmitter stream(@RequestBody ChatRequest request, HttpServletResponse response) {
        response.setHeader("Cache-Control", "no-cache");
        response.setHeader("X-Accel-Buffering", "no");
        SseEmitter emitter = new SseEmitter(180_000L);
        streamExecutor.execute(() -> runStream(request, emitter));
        return emitter;
    }

    private void runStream(ChatRequest request, SseEmitter emitter) {
        SessionContext.set(request.sessionId());
        try {
            if (!llmEnabled) {
                ChatResponse reply = ruleBasedAgentService.handle(request.sessionId(), request.message());
                streamPlainText(emitter, reply.reply() == null ? "" : reply.reply());
                sendEvent(emitter, "done", new StreamDone(reply.actionId(), reply.status()));
                emitter.complete();
                return;
            }
            log.info("chat stream start session={}", request.sessionId());
            llmAgentChatService.stream(request.sessionId(), request.message())
                    .toIterable()
                    .forEach(chunk -> {
                        if (chunk != null && !chunk.isEmpty()) {
                            sendEvent(emitter, "delta", chunk);
                        }
                    });
            ChatResponse wrapped = withPendingAction(request.sessionId(), "");
            sendEvent(emitter, "done", new StreamDone(wrapped.actionId(), wrapped.status()));
            emitter.complete();
        } catch (Exception e) {
            log.warn("chat stream failed: {}", e.getMessage());
            try {
                sendEvent(emitter, "error", java.util.Map.of("message",
                        e.getMessage() == null ? "stream failed" : e.getMessage()));
            } catch (Exception ignored) {
                // emitter already dead
            }
            emitter.completeWithError(e);
        } finally {
            SessionContext.clear();
        }
    }

    private void streamPlainText(SseEmitter emitter, String text) {
        if (text.isEmpty()) {
            return;
        }
        int i = 0;
        while (i < text.length()) {
            int end = Math.min(text.length(), i + 8);
            sendEvent(emitter, "delta", text.substring(i, end));
            i = end;
            try {
                Thread.sleep(12);
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
                return;
            }
        }
    }

    private ChatResponse withPendingAction(String sessionId, String reply) {
        AgentAction pending = actionRepository
                .findFirstBySessionIdAndStatusOrderByCreatedAtDesc(sessionId, ActionStatus.PLANNED)
                .orElse(null);
        if (pending != null) {
            return new ChatResponse(reply, pending.getActionId(), pending.getStatus().name(), null);
        }
        return ChatResponse.text(reply);
    }

    private void sendEvent(SseEmitter emitter, String name, Object payload) {
        try {
            emitter.send(SseEmitter.event().name(name).data(objectMapper.writeValueAsString(payload)));
        } catch (IOException e) {
            throw new IllegalStateException("sse send failed", e);
        }
    }

    private static final class StreamThreadFactory implements ThreadFactory {
        private final AtomicInteger n = new AtomicInteger();

        @Override
        public Thread newThread(Runnable r) {
            Thread t = new Thread(r, "chat-stream-" + n.incrementAndGet());
            t.setDaemon(true);
            return t;
        }
    }
}
