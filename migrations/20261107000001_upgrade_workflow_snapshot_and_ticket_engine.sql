-- +goose Up
-- 1. ticket 表补充 engine_type，存量工单全部默认归属 easy-workflow
DROP PROCEDURE IF EXISTS AddTicketEngineSafe;

-- +goose StatementBegin
CREATE PROCEDURE AddTicketEngineSafe()
BEGIN
    DECLARE CONTINUE HANDLER FOR 1060 BEGIN END; -- 列已存在时忽略
    DECLARE CONTINUE HANDLER FOR 1146 BEGIN END; -- 表不存在时忽略
    ALTER TABLE `ticket`
        ADD COLUMN `engine_type` VARCHAR(32) NOT NULL DEFAULT 'easy-workflow'
        COMMENT '流程引擎类型(easy-workflow/flowx)' AFTER `workflow_id`;
END;
-- +goose StatementEnd

CALL AddTicketEngineSafe();
DROP PROCEDURE IF EXISTS AddTicketEngineSafe;

-- 2. workflow_snapshot 表加固 engine_type 并升级唯一索引为 (workflow_id, engine_type, process_version)
DROP PROCEDURE IF EXISTS UpgradeSnapshotIndexSafe;

-- +goose StatementBegin
CREATE PROCEDURE UpgradeSnapshotIndexSafe()
BEGIN
    DECLARE CONTINUE HANDLER FOR 1060 BEGIN END; -- 列已存在时忽略
    DECLARE CONTINUE HANDLER FOR 1061 BEGIN END; -- 索引已存在时忽略
    DECLARE CONTINUE HANDLER FOR 1091 BEGIN END; -- 索引或列不存在时忽略
    DECLARE CONTINUE HANDLER FOR 1146 BEGIN END; -- 表不存在时忽略

    ALTER TABLE `workflow_snapshot`
        ADD COLUMN `engine_type` VARCHAR(32) NOT NULL DEFAULT 'easy-workflow'
        COMMENT '工作流引擎类型(easy-workflow/flowx)' AFTER `process_version`;

    ALTER TABLE `workflow_snapshot` DROP INDEX `uix_wf_process_version`;
    ALTER TABLE `workflow_snapshot`
        ADD UNIQUE INDEX `uix_wf_engine_version` (`workflow_id`, `engine_type`, `process_version`);
END;
-- +goose StatementEnd

CALL UpgradeSnapshotIndexSafe();
DROP PROCEDURE IF EXISTS UpgradeSnapshotIndexSafe;

-- +goose Down
-- 回滚 ticket 与 workflow_snapshot
DROP PROCEDURE IF EXISTS RollbackEngineAndSnapshotSafe;

-- +goose StatementBegin
CREATE PROCEDURE RollbackEngineAndSnapshotSafe()
BEGIN
    DECLARE CONTINUE HANDLER FOR 1061 BEGIN END; -- 索引已存在时忽略
    DECLARE CONTINUE HANDLER FOR 1091 BEGIN END; -- 索引或列不存在时忽略
    DECLARE CONTINUE HANDLER FOR 1146 BEGIN END; -- 表不存在时忽略

    ALTER TABLE `workflow_snapshot` DROP INDEX `uix_wf_engine_version`;
    ALTER TABLE `workflow_snapshot`
        ADD UNIQUE INDEX `uix_wf_process_version` (`workflow_id`, `process_id`, `process_version`);
    ALTER TABLE `ticket` DROP COLUMN `engine_type`;
END;
-- +goose StatementEnd

CALL RollbackEngineAndSnapshotSafe();
DROP PROCEDURE IF EXISTS RollbackEngineAndSnapshotSafe;
