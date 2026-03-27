-- 脚本包：入口文件名（Agent 执行时的 Python 入口，与 zip 内实际文件名一致）
ALTER TABLE t_agent_script_package
    ADD COLUMN entry_file VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'entry python filename, e.g. szlcsc_crawler.py' AFTER filename;
