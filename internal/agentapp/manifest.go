package agentapp

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	v1 "caichip/api/agent/v1"
)

// ScanInstalledScripts 扫描 data_dir，仅上报 **ready** 的脚本（供任务心跳 §2 installed_scripts）。
func ScanInstalledScripts(dataDir string) ([]*v1.InstalledScript, error) {
	rows, err := ScanScriptRowsForSync(dataDir)
	if err != nil {
		return nil, err
	}
	var out []*v1.InstalledScript
	for _, r := range rows {
		if r.GetEnvStatus() != "ready" {
			continue
		}
		out = append(out, &v1.InstalledScript{
			ScriptId:  r.GetScriptId(),
			Version:   r.GetVersion(),
			EnvStatus: "ready",
		})
	}
	return out, nil
}

// ScanScriptRowsForSync 扫描本地脚本包目录，生成 §3 脚本安装心跳的 scripts[]（含 preparing / failed / ready）。
func ScanScriptRowsForSync(dataDir string) ([]*v1.ScriptRow, error) {
	var rows []*v1.ScriptRow
	_ = os.MkdirAll(dataDir, 0755)
	ents, err := os.ReadDir(dataDir)
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		scriptID := e.Name()
		sub := filepath.Join(dataDir, scriptID)
		vers, err := os.ReadDir(sub)
		if err != nil {
			continue
		}
		for _, ve := range vers {
			if !ve.IsDir() {
				continue
			}
			vdir := filepath.Join(sub, ve.Name())
			row, ok := scriptRowFromVersionDir(scriptID, ve.Name(), vdir)
			if !ok {
				continue
			}
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func scriptRowFromVersionDir(scriptID, dirName, vdir string) (*v1.ScriptRow, bool) {
	if _, err := os.Stat(filepath.Join(vdir, MarkerPreparing)); err == nil {
		ver := readVersionForRow(vdir, dirName)
		return &v1.ScriptRow{
			ScriptId:  scriptID,
			Version:   ver,
			EnvStatus: "preparing",
		}, true
	}
	if b, err := os.ReadFile(filepath.Join(vdir, MarkerEnvFailed)); err == nil {
		ver := readVersionForRow(vdir, dirName)
		return &v1.ScriptRow{
			ScriptId:  scriptID,
			Version:   ver,
			EnvStatus: "failed",
			Message:   strings.TrimSpace(string(b)),
		}, true
	}
	b, err := os.ReadFile(filepath.Join(vdir, "version.txt"))
	if err != nil {
		return nil, false
	}
	raw := strings.TrimSpace(string(b))
	if raw == "" {
		return nil, false
	}
	row := &v1.ScriptRow{
		ScriptId:  scriptID,
		Version:   raw,
		EnvStatus: "ready",
	}
	if bs, err := os.ReadFile(filepath.Join(vdir, FilePackageSHA256)); err == nil {
		s := strings.TrimSpace(string(bs))
		if s != "" {
			row.PackageSha256 = &s
		}
	}
	if st, err := os.Stat(filepath.Join(vdir, "version.txt")); err == nil {
		t := st.ModTime().UTC().Format(time.RFC3339Nano)
		row.InstalledAt = &t
	}
	return row, true
}

func readVersionForRow(vdir, dirName string) string {
	if b, err := os.ReadFile(filepath.Join(vdir, "version.txt")); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" {
			return s
		}
	}
	return dirName
}
