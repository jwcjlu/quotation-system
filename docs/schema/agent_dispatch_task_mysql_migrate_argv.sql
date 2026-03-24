-- 已有 caichip_dispatch_task 时追加 argv_json（与代码 Enqueue/Pull 对齐）。执行一次即可。
ALTER TABLE caichip_dispatch_task
    ADD COLUMN argv_json JSON NULL COMMENT 'string[]，追加到入口脚本后的命令行参数'
    AFTER params_json;
