-- +goose Up
-- SQL in this section is executed when the migration is applied.
ALTER TABLE `task` MODIFY COLUMN `result` MEDIUMTEXT NULL COMMENT '执行输出日志/返回结果';

-- +goose Down
-- SQL in this section is executed when the migration is rolled back.
ALTER TABLE `task` MODIFY COLUMN `result` TEXT NULL COMMENT '执行输出日志/返回结果';
