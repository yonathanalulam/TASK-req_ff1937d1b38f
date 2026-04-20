ALTER TABLE tickets
  DROP FOREIGN KEY fk_ticket_agent,
  DROP INDEX idx_ticket_agent,
  DROP COLUMN assigned_agent_id;
