package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type task struct {
	ID, Stage, Status string
	Dependencies      []string
}

var taskIDRe = regexp.MustCompile(`^[A-Z]+-[0-9]{3}$`)
var taskStates = []string{"待办", "进行中", "待验证", "阻塞", "完成", "暂缓"}
var evidenceHeadings = []string{"## 基线", "## 变更", "## 验证", "## 剩余与交接", "## 结论"}

// checkTasks 校验唯一看板；不扫描历史归档，不从任务卡复制状态。
func checkTasks(root string) ([]task, []problem) {
	const board = "docs/TASKS.md"
	body, err := os.ReadFile(filepath.Join(root, board))
	if err != nil {
		return nil, []problem{{board, err.Error()}}
	}
	var tasks []task
	var problems []problem
	byID := map[string]task{}
	add := func(id, msg string) { problems = append(problems, problem{board, id + ": " + msg}) }
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, "| ") {
			continue
		}
		cells := strings.Split(strings.Trim(line, "|"), "|")
		if len(cells) == 0 {
			continue
		}
		id := strings.TrimSpace(cells[0])
		if id == "ID" || strings.HasPrefix(id, "---") {
			continue
		}
		if !taskIDRe.MatchString(id) {
			add(id, "任务 ID 应为 PREFIX-NNN")
			continue
		}
		if len(cells) != 7 {
			add(id, "任务表必须有七列")
			continue
		}
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		t := task{ID: id, Stage: cells[1], Status: cells[3]}
		if cells[4] != "—" {
			for _, d := range strings.Split(cells[4], ",") {
				t.Dependencies = append(t.Dependencies, strings.TrimSpace(d))
			}
		}
		if _, exists := byID[id]; exists {
			add(id, "任务 ID 重复")
		}
		byID[id] = t
		tasks = append(tasks, t)
		valid := false
		for _, state := range taskStates {
			if t.Status == state {
				valid = true
			}
		}
		if !valid {
			add(id, "非法任务状态")
		}
		if !regexp.MustCompile(`^M[0-9]+$`).MatchString(t.Stage) {
			add(id, "阶段应为 M 加数字")
		}
		if t.Status != "待办" && t.Status != "暂缓" && (cells[5] == "—" || cells[5] == "") {
			add(id, "非待办任务需要负责人")
		}
		// 要求每个任务链接到自己的显式锚点，避免同名标题误跳。
		cardRe := regexp.MustCompile(`^\[[^\]]+\]\((tasks/[^)#]+\.md)#` + strings.ToLower(id) + `\)$`)
		card := cardRe.FindStringSubmatch(cells[2])
		if card == nil {
			add(id, "缺少 tasks/ 下的任务卡及 ID 锚点")
		} else {
			content, e := os.ReadFile(filepath.Join(root, "docs", card[1]))
			if e != nil {
				add(id, "任务卡无法读取")
			} else {
				anchor := `<a id="` + strings.ToLower(id) + `"></a>`
				_, section, found := strings.Cut(string(content), anchor)
				if !found {
					add(id, "任务卡缺少 ID 锚点")
				} else {
					section, _, _ = strings.Cut(section, `<a id="`)
					if !strings.Contains(section, "**前置任务**："+cells[4]+"。") {
						add(id, "任务卡前置任务与看板不一致")
					}
				}
			}
		}
		needsEvidence := t.Status == "完成" || t.Status == "阻塞" || t.Status == "待验证"
		if cells[6] != "—" || needsEvidence {
			expected := "development/evidence/" + id + ".md"
			matches := linkRe.FindStringSubmatch(cells[6])
			if matches == nil || matches[1] != expected {
				add(id, "证据必须链接到 "+expected)
				continue
			}
			content, e := os.ReadFile(filepath.Join(root, "docs", expected))
			if e != nil {
				add(id, "证据文件无法读取")
				continue
			}
			for _, heading := range evidenceHeadings {
				if !strings.Contains(string(content), heading) {
					add(id, "证据缺少 "+heading)
				}
			}
			if t.Status == "完成" && strings.Contains(string(content), "待填写") {
				add(id, "完成证据仍有模板占位符")
			}
		}
	}
	if len(tasks) == 0 {
		add("看板", "未扫描到任务，检查未生效")
	}
	for _, t := range tasks {
		seen := map[string]bool{}
		for _, dep := range t.Dependencies {
			d, ok := byID[dep]
			if !ok {
				add(t.ID, "依赖不存在: "+dep)
			}
			if seen[dep] {
				add(t.ID, "依赖重复: "+dep)
			}
			seen[dep] = true
			if t.Status == "完成" && ok && d.Status != "完成" {
				add(t.ID, "依赖未完成: "+dep)
			}
		}
	}
	colors := map[string]int{}
	var visit func(string)
	visit = func(id string) {
		if colors[id] == 1 {
			add(id, "存在循环依赖")
			return
		}
		if colors[id] == 2 {
			return
		}
		colors[id] = 1
		for _, dep := range byID[id].Dependencies {
			if _, ok := byID[dep]; ok {
				visit(dep)
			}
		}
		colors[id] = 2
	}
	for _, t := range tasks {
		visit(t.ID)
	}
	return tasks, problems
}

func printProgress(tasks []task) {
	groups := map[string]map[string]int{}
	byID := map[string]task{}
	totalDone := 0
	for _, t := range tasks {
		byID[t.ID] = t
		if groups[t.Stage] == nil {
			groups[t.Stage] = map[string]int{}
		}
		groups[t.Stage][t.Status]++
		if t.Status == "完成" {
			totalDone++
		}
	}
	stages := make([]string, 0, len(groups))
	for stage := range groups {
		stages = append(stages, stage)
	}
	sort.Strings(stages)
	fmt.Printf("\n任务数量进度：%d/%d 完成（不代表工作量或产品成熟度）\n", totalDone, len(tasks))
	for _, stage := range stages {
		fmt.Printf("%s", stage)
		for _, state := range taskStates {
			fmt.Printf("  %s:%d", state, groups[stage][state])
		}
		fmt.Println()
	}
	var ready []string
	for _, t := range tasks {
		if t.Status != "待办" {
			continue
		}
		satisfied := true
		for _, dep := range t.Dependencies {
			if byID[dep].Status != "完成" {
				satisfied = false
			}
		}
		if satisfied {
			ready = append(ready, t.ID)
		}
	}
	fmt.Printf("可领取：%s\n", strings.Join(ready, ", "))
}
