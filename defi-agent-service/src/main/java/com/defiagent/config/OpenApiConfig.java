package com.defiagent.config;

import io.swagger.v3.oas.models.OpenAPI;
import io.swagger.v3.oas.models.info.Info;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class OpenApiConfig {

    @Bean
    public OpenAPI defiAgentOpenAPI() {
        return new OpenAPI().info(new Info()
                .title("DeFi AI Agent API")
                .version("0.1.0")
                .description("Natural language -> plan -> async (paper) execution. "
                        + "Spring Boot + MySQL + Redis + Kafka."));
    }
}
