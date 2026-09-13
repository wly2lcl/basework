package main

import (
	"fmt"
	"sort"

	"github.com/wly2lcl/basework/internal/jobs"
	"github.com/wly2lcl/basework/internal/tui"
	"github.com/wly2lcl/basework/pkg/session"
)

// collectResumeItems 收集「重启后可恢复进度」的三类事实（UI-003）：
//  1. 后台任务：interrupted/failed → 需重试/复核（重启归并已由 JOB-004 保证）；
//  2. 最近修改：工作区事实里的 file_modified（CTX-001）；
//  3. 验证摘要：最后一次带验证命令的编辑事件——改了什么、怎么验证，
//     重启后一眼可查。
//
// 任何一路失败都不阻塞其余两路：恢复面板是帮助，不是门禁。
func collectResumeItems(mgr *jobs.Manager, owner string, store *session.JSONLStore, sessionID, wsID string) []tui.ResumeItem {
	var items []tui.ResumeItem

	// --- 1. 后台任务 ---
	if mgr != nil {
		hist, err := mgr.History(owner)
		if err == nil {
			for _, j := range hist {
				needsRetry := false
				switch j.State {
				case jobs.StateInterrupted:
					needsRetry = true // 进程退出时还在跑：结果未知，需重跑
				case jobs.StateFailed:
					needsRetry = true // 失败：需复核原因后重试
				}
				items = append(items, tui.ResumeItem{
					Kind:       "job",
					Label:      j.Command,
					State:      string(j.State),
					NeedsRetry: needsRetry,
					Ref:        j.ID,
				})
			}
		}
	}

	// --- 2. 最近修改 ---
	if w, err := session.LoadWorkspaceFacts(getSessionDir(), wsID); err == nil && w != nil {
		files := make([]session.Fact, 0)
		for _, f := range w.List() {
			if f.Kind == session.FactFileModified {
				files = append(files, f)
			}
		}
		sort.Slice(files, func(i, j int) bool { return files[i].FirstSeen > files[j].FirstSeen })
		for i, f := range files {
			if i >= 8 {
				break
			}
			items = append(items, tui.ResumeItem{
				Kind:  "file",
				Label: f.Path,
				State: "已修改",
				Ref:   f.Ref,
			})
		}
	}

	// --- 3. 验证摘要 ---
	if store != nil && sessionID != "" {
		events, err := store.Events(session.EventFilter{
			SessionID: sessionID,
			Types:     []session.EventType{session.EventFileEdited},
			Limit:     50,
		})
		if err == nil {
			var lastVerify, lastPlan string
			for _, ev := range events {
				decoded, err := session.DecodeData(ev)
				if err != nil {
					continue
				}
				data, ok := decoded.(*session.FileEditedData)
				if !ok || data.Verify == "" || data.Phase != "committed" {
					continue
				}
				lastVerify, lastPlan = data.Verify, data.PlanID
			}
			if lastVerify != "" {
				items = append(items, tui.ResumeItem{
					Kind:  "verify",
					Label: fmt.Sprintf("上次验证命令 %s（计划 %s）——重启后建议重跑", lastVerify, lastPlan),
					State: "待确认",
					Ref:   lastPlan,
				})
			}
		}
	}

	return items
}
