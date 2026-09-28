package com.defiagent.cli;

import com.fasterxml.jackson.databind.DeserializationFeature;
import com.fasterxml.jackson.databind.ObjectMapper;
import com.fasterxml.jackson.databind.PropertyNamingStrategies;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.context.annotation.Primary;
import org.springframework.http.converter.json.Jackson2ObjectMapperBuilder;

/**
 * Dedicated ObjectMapper for parsing defi-cli's snake_case JSON envelope into
 * our camelCase Java records, kept separate from the app-wide Jackson config
 * used for REST/JPA/Redis so this mapping choice doesn't leak elsewhere.
 */
@Configuration
public class CliJacksonConfig {

    /**
     * Re-declares Spring Boot's own default {@code ObjectMapper} explicitly, and
     * marks it {@code @Primary}.
     *
     * <p><b>Why this is needed:</b> {@code JacksonAutoConfiguration}'s own default
     * {@code ObjectMapper} bean is annotated {@code @ConditionalOnMissingBean(ObjectMapper.class)}.
     * Defining {@code defiCliObjectMapper} below — a plain {@code ObjectMapper}-typed
     * bean, regardless of its name/qualifier — satisfies that condition and silently
     * suppresses Boot's own bean, leaving {@code defiCliObjectMapper} as the *only*
     * {@code ObjectMapper} in the context. Spring MVC's
     * {@code MappingJackson2HttpMessageConverter} then picks it up for every REST
     * response — a snake_case, no-JSR310-module mapper — so any endpoint returning a
     * {@code java.time.Instant} (or any other Boot Java-8-time type, e.g.
     * {@code AgentAction.createdAt}) 500s with
     * {@code HttpMessageConversionException: Type definition error: [simple type, class java.time.Instant]}.
     * This bean restores the normal Boot-built mapper (JSR310 module, camelCase,
     * etc.) and marks it {@code @Primary} so it — not the CLI-specific one — is what
     * gets autowired everywhere an unqualified {@code ObjectMapper} is requested.
     */
    @Bean
    @Primary
    public ObjectMapper objectMapper(Jackson2ObjectMapperBuilder builder) {
        return builder.build();
    }

    @Bean("defiCliObjectMapper")
    public ObjectMapper defiCliObjectMapper() {
        return new ObjectMapper()
                .setPropertyNamingStrategy(PropertyNamingStrategies.SNAKE_CASE)
                .configure(DeserializationFeature.FAIL_ON_UNKNOWN_PROPERTIES, false);
    }
}
