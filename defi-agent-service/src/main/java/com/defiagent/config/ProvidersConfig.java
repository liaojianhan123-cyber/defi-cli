package com.defiagent.config;

import com.defiagent.yield.MockYieldProvider;
import com.defiagent.yield.YieldDataProvider;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Registers the mock {@link YieldDataProvider} fallback beans used by
 * {@link com.defiagent.yield.YieldService} only when the real
 * {@link com.defiagent.cli.DefiCliClient} is unavailable. Real data now comes
 * from defi-cli directly (see {@code /api/v1/yield/*} and {@code DefiToolService}).
 */
@Configuration
public class ProvidersConfig {

    @Bean
    public YieldDataProvider aaveProvider() {
        return new MockYieldProvider("aave", 3.2, 1_200_000_000d);
    }

    @Bean
    public YieldDataProvider morphoProvider() {
        return new MockYieldProvider("morpho", 4.1, 350_000_000d);
    }

    @Bean
    public YieldDataProvider compoundProvider() {
        return new MockYieldProvider("compoundv3", 3.7, 600_000_000d);
    }
}
