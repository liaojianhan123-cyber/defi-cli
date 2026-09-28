package com.defiagent.yield;

import com.defiagent.cli.DefiCliClient;
import com.defiagent.cli.DefiCliException;
import com.fasterxml.jackson.core.type.TypeReference;
import com.fasterxml.jackson.databind.ObjectMapper;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Qualifier;
import org.springframework.data.redis.core.StringRedisTemplate;
import org.springframework.stereotype.Service;

import java.time.Duration;
import java.util.Comparator;
import java.util.List;
import java.util.Objects;
import java.util.concurrent.CompletableFuture;
import java.util.concurrent.Executor;

/**
 * Ranks yield opportunities for the natural-language chat/deposit flow.
 *
 * <p>Primary data source is the real {@link DefiCliClient} (defi-cli aggregates
 * Aave/Morpho/Compound V3/Moonwell/Pendle etc. itself). If defi-cli is
 * unavailable (binary missing, network down, provider error), this service
 * degrades to the deterministic {@link YieldDataProvider} mocks so the chat
 * flow keeps working offline/in demos — a graceful-degradation pattern, not
 * just a placeholder.
 *
 * <p>Demonstrates two JD keywords in the fallback path:
 * <ul>
 *   <li><b>Multithreading</b>: mock providers are queried in parallel on {@code yieldExecutor}.</li>
 *   <li><b>Redis caching</b>: the ranked result (from either source) is cached (cache-aside) with a short TTL.</li>
 * </ul>
 */
@Service
public class YieldService {

    private static final Logger log = LoggerFactory.getLogger(YieldService.class);
    private static final Duration CACHE_TTL = Duration.ofSeconds(60);

    private final DefiCliClient cli;
    private final List<YieldDataProvider> mockProviders;
    private final Executor executor;
    private final StringRedisTemplate redis;
    private final ObjectMapper objectMapper;

    public YieldService(DefiCliClient cli,
                        List<YieldDataProvider> mockProviders,
                        @Qualifier("yieldExecutor") Executor executor,
                        StringRedisTemplate redis,
                        ObjectMapper objectMapper) {
        this.cli = cli;
        this.mockProviders = mockProviders;
        this.executor = executor;
        this.redis = redis;
        this.objectMapper = objectMapper;
    }

    public List<YieldOpportunity> topOpportunities(String asset, String chain, int limit) {
        String cacheKey = "yield:" + asset + ":" + chain;

        List<YieldOpportunity> cached = readCache(cacheKey);
        if (cached != null) {
            log.debug("yield cache hit for {}", cacheKey);
            return cached.stream().limit(limit).toList();
        }

        List<YieldOpportunity> ranked = fetchFromCli(asset, chain, limit);
        if (ranked.isEmpty()) {
            log.warn("defi-cli returned no usable yield opportunities for asset={} chain={}; falling back to mock providers",
                    asset, chain);
            ranked = fetchFromMockProviders(asset, chain);
        }

        writeCache(cacheKey, ranked);
        return ranked.stream().limit(limit).toList();
    }

    private List<YieldOpportunity> fetchFromCli(String asset, String chain, int limit) {
        try {
            return cli.yieldOpportunities(chain, asset, null, Math.max(limit, 10), null, null, "apy_total")
                    .stream()
                    .map(o -> new YieldOpportunity(
                            o.provider(),
                            chain,
                            asset,
                            o.apyTotal() == null ? 0.0 : o.apyTotal(),
                            o.tvlUsd() == null ? 0.0 : o.tvlUsd()))
                    .sorted(Comparator.comparingDouble(YieldOpportunity::apy).reversed())
                    .toList();
        } catch (DefiCliException e) {
            log.warn("defi-cli yield opportunities failed (code={} type={}): {}", e.getCode(), e.getType(), e.getMessage());
            return List.of();
        } catch (Exception e) {
            log.warn("defi-cli yield opportunities failed unexpectedly: {}", e.getMessage());
            return List.of();
        }
    }

    private List<YieldOpportunity> fetchFromMockProviders(String asset, String chain) {
        // Fan out to all mock providers in parallel, then join.
        List<CompletableFuture<YieldOpportunity>> futures = mockProviders.stream()
                .map(p -> CompletableFuture.supplyAsync(() -> safeFetch(p, asset, chain), executor))
                .toList();

        return futures.stream()
                .map(CompletableFuture::join)
                .filter(Objects::nonNull)
                .sorted(Comparator.comparingDouble(YieldOpportunity::apy).reversed())
                .toList();
    }

    private YieldOpportunity safeFetch(YieldDataProvider p, String asset, String chain) {
        try {
            return p.fetch(asset, chain);
        } catch (Exception e) {
            log.warn("mock provider {} failed: {}", p.name(), e.getMessage());
            return null;
        }
    }

    private List<YieldOpportunity> readCache(String key) {
        try {
            String json = redis.opsForValue().get(key);
            if (json == null) {
                return null;
            }
            return objectMapper.readValue(json, new TypeReference<List<YieldOpportunity>>() {
            });
        } catch (Exception e) {
            log.warn("yield cache read failed for {}: {}", key, e.getMessage());
            return null;
        }
    }

    private void writeCache(String key, List<YieldOpportunity> value) {
        try {
            redis.opsForValue().set(key, objectMapper.writeValueAsString(value), CACHE_TTL);
        } catch (Exception e) {
            log.warn("yield cache write failed for {}: {}", key, e.getMessage());
        }
    }
}
