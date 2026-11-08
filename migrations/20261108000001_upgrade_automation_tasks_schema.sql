-- +goose Up
-- 1. 升级 automation_tasks 表：新增 instance_key，存量历史数据平滑刷盘，解耦引擎强绑定
DROP PROCEDURE IF EXISTS UpgradeAutomationTasksSafe;

-- +goose StatementBegin
CREATE PROCEDURE UpgradeAutomationTasksSafe()
BEGIN
    DECLARE CONTINUE HANDLER FOR 1060 BEGIN END; -- 列已存在时忽略
    DECLARE CONTINUE HANDLER FOR 1061 BEGIN END; -- 索引已存在时忽略
    DECLARE CONTINUE HANDLER FOR 1091 BEGIN END; -- 索引或列不存在时忽略
    DECLARE CONTINUE HANDLER FOR 1146 BEGIN END; -- 表不存在时忽略

    -- 1.1 新增字符串形态的实例全局标识 Key
    ALTER TABLE `automation_tasks`
        ADD COLUMN `instance_key` VARCHAR(64) NOT NULL DEFAULT ''
        COMMENT '流程实例全局唯一标识Key' AFTER `process_instance_id`;

    -- 1.2 存量数据刷盘：将老 process_instance_id 数字平滑转为 instance_key
    UPDATE `automation_tasks`
        SET `instance_key` = CAST(`process_instance_id` AS CHAR)
        WHERE `instance_key` = '' AND `process_instance_id` > 0;

    -- 1.3 建立以 (tenant_id, ticket_id, node_id) 为核心的业务权威唯一索引
    ALTER TABLE `automation_tasks`
        ADD UNIQUE KEY `uk_tenant_ticket_node` (`tenant_id`, `ticket_id`, `node_id`);

    -- 1.4 旧唯一索引 uk_automation_node 降级为普通辅助查询索引
    ALTER TABLE `automation_tasks` DROP INDEX `uk_automation_node`;
    ALTER TABLE `automation_tasks`
        ADD INDEX `idx_tenant_process_node` (`tenant_id`, `process_instance_id`, `node_id`);

    -- 1.5 清理未落地与冗余死字段
    ALTER TABLE `automation_tasks` DROP COLUMN `original_scheduled_at`;
    ALTER TABLE `automation_tasks` DROP COLUMN `cancelled_at`;
    ALTER TABLE `automation_tasks` DROP COLUMN `phase`;
END;
-- +goose StatementEnd

CALL UpgradeAutomationTasksSafe();
DROP PROCEDURE IF EXISTS UpgradeAutomationTasksSafe;

-- +goose Down
DROP PROCEDURE IF EXISTS RollbackAutomationTasksSafe;

-- +goose StatementBegin
CREATE PROCEDURE RollbackAutomationTasksSafe()
BEGIN
    DECLARE CONTINUE HANDLER FOR 1060 BEGIN END;
    DECLARE CONTINUE HANDLER FOR 1061 BEGIN END;
    DECLARE CONTINUE HANDLER FOR 1091 BEGIN END;
    DECLARE CONTINUE HANDLER FOR 1146 BEGIN END;

    -- 回滚字段
    ALTER TABLE `automation_tasks`
        ADD COLUMN `original_scheduled_at` BIGINT NOT NULL DEFAULT 0 COMMENT '首次计划提交时间';
    ALTER TABLE `automation_tasks`
        ADD COLUMN `cancelled_at` BIGINT NOT NULL DEFAULT 0 COMMENT '任务取消或强制终止时间';
    ALTER TABLE `automation_tasks`
        ADD COLUMN `phase` VARCHAR(32) NOT NULL DEFAULT 'READY' COMMENT '最近编排阶段';

    -- 回滚索引
    ALTER TABLE `automation_tasks` DROP INDEX `uk_tenant_ticket_node`;
    ALTER TABLE `automation_tasks` DROP INDEX `idx_tenant_process_node`;
    ALTER TABLE `automation_tasks`
        ADD UNIQUE KEY `uk_automation_node` (`tenant_id`, `process_instance_id`, `node_id`);

    ALTER TABLE `automation_tasks` DROP COLUMN `instance_key`;
END;
-- +goose StatementEnd

CALL RollbackAutomationTasksSafe();
DROP PROCEDURE IF EXISTS RollbackAutomationTasksSafe;
