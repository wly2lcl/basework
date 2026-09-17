//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strconv"
)

func configureTestProcess(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		pid := cmd.Process.Pid
		// taskkill /T only waits for the root process. A descendant can still
		// hold the verification directory open after cmd.Run returns, which
		// makes TempDir cleanup fail on Windows. Snapshot and terminate the
		// complete tree in PowerShell, then wait until every captured PID is
		// gone before returning to os/exec.
		if err := terminateWindowsProcessTree(pid); err == nil {
			return nil
		}
		// Keep taskkill as a fallback for restricted PowerShell environments.
		return exec.Command("taskkill", "/PID", strconv.Itoa(pid), "/T", "/F").Run()
	}
}

func terminateWindowsProcessTree(rootPID int) error {
	const script = `$root = %d
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
$ids.Add($root)
for ($i = $ids.Count - 1; $i -ge 0; $i--) {
  Stop-Process -Id $ids[$i] -Force -ErrorAction SilentlyContinue
}
$deadline = (Get-Date).AddSeconds(2)
do {
  $alive = @($ids | ForEach-Object { Get-Process -Id $_ -ErrorAction SilentlyContinue })
  if ($alive.Count -eq 0) { exit 0 }
  Start-Sleep -Milliseconds 25
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
