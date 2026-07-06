package agent

import (
	"os"
	"strings"
	"testing"
)

func TestWorkdirProviderName(t *testing.T) {
	p := &workdirProvider{}
	if p.Name() != "workdir" {
		t.Errorf("Name() = %q, 期望 'workdir'", p.Name())
	}
}

func TestWorkdirProviderCollect(t *testing.T) {
	p := &workdirProvider{}
	text, err := p.Collect()
	if err != nil {
		t.Fatalf("Collect() 失败: %v", err)
	}
	if text == "" {
		t.Fatal("Collect() 返回空")
	}
	if !strings.Contains(text, "Working directory:") {
		t.Errorf("返回值应包含 'Working directory:', 得到 %q", text)
	}
	cwd, _ := os.Getwd()
	if !strings.Contains(text, cwd) {
		t.Errorf("返回值应包含当前目录 %q, 得到 %q", cwd, text)
	}
}

func TestGitProviderName(t *testing.T) {
	p := &gitProvider{}
	if p.Name() != "git" {
		t.Errorf("Name() = %q, 期望 'git'", p.Name())
	}
}

func TestGitProviderCollect(t *testing.T) {
	p := &gitProvider{}
	text, err := p.Collect()
	if err != nil {
		t.Fatalf("Collect() 失败: %v", err)
	}
	if !strings.Contains(text, "Git branch:") {
		t.Errorf("在 git 仓库中应包含分支信息, 得到 %q", text)
	}
}

func TestGitProviderCollectNoGitDir(t *testing.T) {
	p := &gitProvider{}
	tmpDir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	text, err := p.Collect()
	if err != nil {
		t.Fatalf("Collect() 失败: %v", err)
	}
	if text != "" {
		t.Logf("非 git 目录返回了文本: %q", text)
	}
}

func TestPlatformProviderName(t *testing.T) {
	p := &platformProvider{}
	if p.Name() != "platform" {
		t.Errorf("Name() = %q, 期望 'platform'", p.Name())
	}
}

func TestPlatformProviderCollect(t *testing.T) {
	p := &platformProvider{}
	text, err := p.Collect()
	if err != nil {
		t.Fatalf("Collect() 失败: %v", err)
	}
	if text == "" {
		t.Fatal("Collect() 返回空")
	}
	if !strings.Contains(text, "Platform:") {
		t.Errorf("应包含 'Platform:', 得到 %q", text)
	}
}

func TestPlatformDetectShell(t *testing.T) {
	p := &platformProvider{}
	shell := p.detectShell()
	if shell == "" {
		t.Error("detectShell() 不应返回空")
	}
}

func TestProjectProviderName(t *testing.T) {
	p := &projectProvider{}
	if p.Name() != "project" {
		t.Errorf("Name() = %q, 期望 'project'", p.Name())
	}
}

func TestProjectProviderCollect(t *testing.T) {
	p := &projectProvider{}
	text, err := p.Collect()
	if err != nil {
		t.Fatalf("Collect() 失败: %v", err)
	}
	if text == "" {
		t.Log("当前目录无项目文件，Collect() 返回空")
	}
}

func TestProjectDetectType(t *testing.T) {
	p := &projectProvider{}
	dir := t.TempDir()

	if typ := p.detectType(dir); typ != "" {
		t.Errorf("空目录期望空, 得到 %q", typ)
	}

	_ = os.WriteFile(dir+"/go.mod", []byte("module test"), 0644)
	if typ := p.detectType(dir); typ != "Go" {
		t.Errorf("期望 'Go', 得到 %q", typ)
	}

	_ = os.Remove(dir + "/go.mod")
	_ = os.WriteFile(dir+"/requirements.txt", []byte(""), 0644)
	if typ := p.detectType(dir); typ != "Python" {
		t.Errorf("期望 'Python', 得到 %q", typ)
	}
}

func TestProjectFindKeyFiles(t *testing.T) {
	p := &projectProvider{}
	dir := t.TempDir()

	files := p.findKeyFiles(dir)
	if len(files) != 0 {
		t.Errorf("空目录期望 0 文件, 得到 %d", len(files))
	}

	_ = os.WriteFile(dir+"/Makefile", []byte(""), 0644)
	_ = os.WriteFile(dir+"/Dockerfile", []byte(""), 0644)

	files = p.findKeyFiles(dir)
	if len(files) != 2 {
		t.Errorf("期望 2 文件, 得到 %d: %v", len(files), files)
	}
}