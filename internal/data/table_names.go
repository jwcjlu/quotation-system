package data

// 业务库物理表名统一前缀 t_（与 docs/schema 下 DDL 一致）。
const (
	TableCaichipDispatchTask         = "t_caichip_dispatch_task"
	TableCaichipAgent                = "t_caichip_agent"
	TableCaichipAgentTag             = "t_caichip_agent_tag"
	TableCaichipAgentInstalledScript = "t_caichip_agent_installed_script"
	TableBomSearchTask               = "t_bom_search_task"
	TableBomQuoteCache               = "t_bom_quote_cache"
	TableBomSession                  = "t_bom_session"
	TableBomSessionLine              = "t_bom_session_line"
	TableBomMergeInflight            = "t_bom_merge_inflight"
	TableBomPlatformScript           = "t_bom_platform_script"
	TableAgentScriptPackage          = "t_agent_script_package"
)
