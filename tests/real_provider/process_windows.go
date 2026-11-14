//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

func configureTestProcess(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		pid := cmd.Process.Pid
		// taskkill is the fast path; its /T traversal is not atomic, so the
		// PowerShell reconciliation below repeats the parent-PID scan until the
		// whole tree has disappeared. This prevents a late-created go test
		// child from keeping the verification directory open on Windows.
		taskkillErr := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
		reconcileErr := reconcileWindowsProcessTree(pid)
		if reconcileErr == nil || taskkillErr == nil {
			return nil
		}
		return taskkillErr
	}
}

func reconcileWindowsProcessTree(rootPID int) error {
	const script = `$root = %d
$deadline = (Get-Date).AddSeconds(5)
do {
  $all = @(Get-CimInstance Win32_Process -ErrorAction SilentlyContinue | Select-Object ProcessId, ParentProcessId)
  $children = @{}
  foreach ($p in $all) {
    $parent = [int]$p.ParentProcessId
    if (-not $children.ContainsKey($parent)) { $children[$parent] = New-Object 'System.Collections.Generic.List[int]' }
    [void]$children[$parent].Add([int]$p.ProcessId)
  }
  $queue = New-Object 'System.Collections.Generic.Queue[int]'
  $seen = New-Object 'System.Collections.Generic.HashSet[int]'
  $ids = New-Object 'System.Collections.Generic.List[int]'
  $queue.Enqueue($root)
  [void]$seen.Add($root)
  while ($queue.Count -gt 0) {
    $parent = $queue.Dequeue()
    if (-not $children.ContainsKey($parent)) { continue }
    foreach ($child in $children[$parent]) {
      if ($seen.Add($child)) {
        $queue.Enqueue($child)
        $ids.Add($child)
      }
    }
  }
  for ($i = $ids.Count - 1; $i -ge 0; $i--) {
    Stop-Process -Id $ids[$i] -Force -ErrorAction SilentlyContinue
  }
  Stop-Process -Id $root -Force -ErrorAction SilentlyContinue
  $alive = @($ids | ForEach-Object { Get-Process -Id $_ -ErrorAction SilentlyContinue })
  $rootAlive = @(Get-Process -Id $root -ErrorAction SilentlyContinue)
  if ($alive.Count -eq 0 -and $rootAlive.Count -eq 0) { exit 0 }
  Start-Sleep -Milliseconds 50
} while ((Get-Date) -lt $deadline)
exit 1`

	return exec.Command(
		"powershell.exe",
		"-NoLogo",
		"-NoProfile",
		"-NonInteractive",
		"-ExecutionPolicy", "Bypass",
		"-Command", fmt.Sprintf(script, rootPID),
	).Run()
}
