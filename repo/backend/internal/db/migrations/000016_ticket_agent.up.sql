-- 000007b: Assigned agent column for tickets

ALTER TABLE tickets
  ADD COLUMN assigned_agent_id BIGINT UNSIGNED NULL AFTER user_id,
  ADD INDEX idx_ticket_agent (assigned_agent_id),
  ADD CONSTRAINT fk_ticket_agent FOREIGN KEY (assigned_agent_id) REFERENCES users (id) ON DELETE SET NULL;
