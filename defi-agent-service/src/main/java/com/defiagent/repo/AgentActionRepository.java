package com.defiagent.repo;

import com.defiagent.domain.ActionStatus;
import com.defiagent.domain.AgentAction;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;
import java.util.Optional;

public interface AgentActionRepository extends JpaRepository<AgentAction, Long> {

    Optional<AgentAction> findByActionId(String actionId);

    Optional<AgentAction> findFirstBySessionIdAndStatusOrderByCreatedAtDesc(String sessionId, ActionStatus status);

    Optional<AgentAction> findByDefiActionId(String defiActionId);

    List<AgentAction> findTop20BySessionIdOrderByCreatedAtDesc(String sessionId);
}
