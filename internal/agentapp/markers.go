package agentapp

// 本地脚本目录状态标记（与协议脚本 sync §3 的 env_status 对应）。
const (
	MarkerPreparing   = ".caichip_preparing"
	MarkerEnvFailed   = ".caichip_env_failed"
	FilePackageSHA256 = ".package_sha256"
)
