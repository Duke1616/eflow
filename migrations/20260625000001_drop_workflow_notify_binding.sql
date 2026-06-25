-- +goose Up
DROP TABLE IF EXISTS workflow_notify_binding;

-- +goose Down
CREATE TABLE IF NOT EXISTS workflow_notify_binding (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  tenant_id BIGINT NOT NULL,
  workflow_id BIGINT NOT NULL,
  notify_type VARCHAR(64) NOT NULL,
  channel VARCHAR(64) NOT NULL,
  template_id BIGINT NOT NULL,
  ctime BIGINT,
  utime BIGINT,
  INDEX idx_workflow_notify_binding_tenant_id (tenant_id),
  INDEX idx_workflow_notify_binding_workflow_id (workflow_id)
);
