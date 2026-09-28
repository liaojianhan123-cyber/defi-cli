package com.defiagent.domain;

import jakarta.persistence.Column;
import jakarta.persistence.Entity;
import jakarta.persistence.EnumType;
import jakarta.persistence.Enumerated;
import jakarta.persistence.GeneratedValue;
import jakarta.persistence.GenerationType;
import jakarta.persistence.Id;
import jakarta.persistence.Index;
import jakarta.persistence.PrePersist;
import jakarta.persistence.PreUpdate;
import jakarta.persistence.Table;
import jakarta.persistence.Version;

import java.time.Instant;

/**
 * A single agent action (plan + execution record).
 * This is the SSOT for the action lifecycle, persisted in MySQL.
 */
@Entity
@Table(
        name = "agent_action",
        indexes = {
                @Index(name = "idx_action_id", columnList = "actionId", unique = true),
                @Index(name = "idx_session_status", columnList = "sessionId,status")
        }
)
public class AgentAction {

    @Id
    @GeneratedValue(strategy = GenerationType.IDENTITY)
    private Long id;

    @Column(nullable = false, unique = true, length = 64)
    private String actionId;

    @Column(nullable = false, length = 64)
    private String sessionId;

    @Column(nullable = false, length = 32)
    private String intent;

    @Column(length = 32)
    private String provider;

    @Column(length = 32)
    private String chain;

    @Column(length = 32)
    private String asset;

    /** Human-readable amount as entered (kept as string to stay decimal-safe). */
    @Column(length = 64)
    private String amount;

    private Double apy;

    private Double tvlUsd;

    @Enumerated(EnumType.STRING)
    @Column(nullable = false, length = 16)
    private ActionStatus status;

    /**
     * JPA optimistic lock. Every {@code UPDATE} checks this value; a concurrent
     * writer that read a stale version gets {@code ObjectOptimisticLockingFailureException}
     * instead of silently overwriting the other writer's change. This is the
     * safety net behind the explicit {@link ActionStatus} transition checks below.
     */
    @Version
    @Column(nullable = false)
    private Long version;

    @Column(length = 80)
    private String txHash;

    /** Real action_id returned by defi-cli's own action store for the plan step (nullable: mock/no-CLI fallback). */
    @Column(length = 64)
    private String defiActionId;

    /** Compact JSON summary of the defi-cli plan's steps, for UI display. */
    @Column(length = 2000)
    private String stepsSummary;

    @Column(length = 500)
    private String note;

    @Column(nullable = false)
    private Instant createdAt;

    @Column(nullable = false)
    private Instant updatedAt;

    @PrePersist
    void onCreate() {
        Instant now = Instant.now();
        this.createdAt = now;
        this.updatedAt = now;
    }

    @PreUpdate
    void onUpdate() {
        this.updatedAt = Instant.now();
    }

    // ── getters / setters ────────────────────────────────────────────────

    public Long getId() {
        return id;
    }

    public String getActionId() {
        return actionId;
    }

    public void setActionId(String actionId) {
        this.actionId = actionId;
    }

    public String getSessionId() {
        return sessionId;
    }

    public void setSessionId(String sessionId) {
        this.sessionId = sessionId;
    }

    public String getIntent() {
        return intent;
    }

    public void setIntent(String intent) {
        this.intent = intent;
    }

    public String getProvider() {
        return provider;
    }

    public void setProvider(String provider) {
        this.provider = provider;
    }

    public String getChain() {
        return chain;
    }

    public void setChain(String chain) {
        this.chain = chain;
    }

    public String getAsset() {
        return asset;
    }

    public void setAsset(String asset) {
        this.asset = asset;
    }

    public String getAmount() {
        return amount;
    }

    public void setAmount(String amount) {
        this.amount = amount;
    }

    public Double getApy() {
        return apy;
    }

    public void setApy(Double apy) {
        this.apy = apy;
    }

    public Double getTvlUsd() {
        return tvlUsd;
    }

    public void setTvlUsd(Double tvlUsd) {
        this.tvlUsd = tvlUsd;
    }

    public ActionStatus getStatus() {
        return status;
    }

    public void setStatus(ActionStatus status) {
        this.status = status;
    }

    /**
     * Moves this action to {@code target}, enforcing {@link ActionStatus#canTransitionTo}.
     * Use this instead of {@link #setStatus} everywhere except the initial PLANNED
     * creation, so an invalid move (e.g. duplicate Kafka delivery re-entering
     * EXECUTING from CONFIRMED) fails loudly instead of corrupting the record.
     *
     * @throws IllegalActionStateException if {@code status -> target} is not a legal transition
     */
    public void transitionTo(ActionStatus target) {
        if (!this.status.canTransitionTo(target)) {
            throw new IllegalActionStateException(this.actionId, this.status, target);
        }
        this.status = target;
    }

    public Long getVersion() {
        return version;
    }

    public String getTxHash() {
        return txHash;
    }

    public void setTxHash(String txHash) {
        this.txHash = txHash;
    }

    public String getDefiActionId() {
        return defiActionId;
    }

    public void setDefiActionId(String defiActionId) {
        this.defiActionId = defiActionId;
    }

    public String getStepsSummary() {
        return stepsSummary;
    }

    public void setStepsSummary(String stepsSummary) {
        this.stepsSummary = stepsSummary;
    }

    public String getNote() {
        return note;
    }

    public void setNote(String note) {
        this.note = note;
    }

    public Instant getCreatedAt() {
        return createdAt;
    }

    public Instant getUpdatedAt() {
        return updatedAt;
    }
}
