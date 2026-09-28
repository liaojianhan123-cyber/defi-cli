package com.defiagent.config;

import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;
import org.springframework.scheduling.concurrent.ThreadPoolTaskExecutor;

import java.util.concurrent.Executor;

/**
 * Thread pool used to query multiple DeFi providers concurrently.
 * Demonstrates bounded multithreading (JD: "multithreading and networking").
 */
@Configuration
public class AsyncConfig {

    @Bean(name = "yieldExecutor")
    public Executor yieldExecutor() {
        ThreadPoolTaskExecutor executor = new ThreadPoolTaskExecutor();
        executor.setCorePoolSize(4);
        executor.setMaxPoolSize(8);
        executor.setQueueCapacity(50);
        executor.setThreadNamePrefix("yield-");
        executor.initialize();
        return executor;
    }
}
