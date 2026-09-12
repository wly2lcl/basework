package session

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
)

// SchemaVersion 是当前会话数据的格式版本。
//
// 递增规则：只要对 Event 结构或其 Data 负载做**不兼容**修改（删除字段、
// 改变字段语义、新增必填字段），就递增本值，并在下方 eventMigrations /
// dbMigrations 中补上从上一个版本到本版本的**相邻**迁移步骤。
//
// 无需递增的兼容变更：新增带 omitempty 的可选字段。
//
// v1 → v2 的理由值得单独说明：本次新增的三种事件（system.prompt_set / steered /
// request.built）不属于对话历史投影，但会影响发给 provider 的请求如何组装。
// 此外，当前投影器会识别 v1 CompactedData 中若已存在的 Summary / TruncatedSeq；
// 因而仅升级版本戳不会改写 payload，却不能保证新版本的投影结果与旧程序相同。
// 「日志里记的 system prompt」与「配置里的 system prompt」也是两个东西：旧版本代码
// 读新版本日志时可能忽略日志上下文，改从配置取值，从而构造与写入方意图不同的请求。
// 这些解释规则变化是版本号要标明的能力边界，所以照样递增。
const SchemaVersion = 2

// ErrSchemaTooNew 表示会话数据的格式版本高于当前程序支持的版本。
//
// 遇到该错误必须**拒绝**而不是尽力解析：同一份日志在不同投影规则下会产出
// 不同历史，静默继续等于制造难以察觉的错误上下文。写入路径同样必须失败，
// 不得降级写成旧格式——那会把错误数据固化下来。
var ErrSchemaTooNew = errors.New("session: 会话格式版本高于当前支持版本")

// migration 表示一次相邻的格式版本升级（vN → vN+1）。
//
// 只允许相邻步进，不允许跨版本直达：N 个相邻步骤只需 N 个测试，
// 而覆盖任意版本组合需要 N² 个。
type migration struct {
	From, To int
	// Apply 升级单条事件的 JSON（JSONL 存储使用）。可为 nil。
	Apply func(raw []byte) ([]byte, error)
	// ApplyDB 升级整个 SQLite 数据库（SQLite 存储使用）。可为 nil。
	ApplyDB func(db *sql.DB) error
}

// eventMigrations 是 JSONL 事件级迁移链，按 From 升序排列。
var eventMigrations = []migration{
	{
		From: 0,
		To:   1,
		// v0 → v1：仅补上格式版本号，事件结构本身未变。
		// 建立机制的成本在「还没有任何不兼容变更」时最低，因此在这里先立起来。
		Apply: func(raw []byte) ([]byte, error) { return stampSchemaVersion(raw, 1) },
	},
	{
		From: 1,
		To:   2,
		// v1 → v2：迁移只补版本戳，不改写事件 payload。新投影器仍会按当前规则
		// 解释 v1 CompactedData 中已有的 Summary / TruncatedSeq（若存在），所以
		// payload 不变不代表投影结果不变。版本号在这里标明读端支持的新解释规则，
		// 以及「请求上下文可能来自日志本身，而不只是配置」这一能力边界。
		Apply: func(raw []byte) ([]byte, error) { return stampSchemaVersion(raw, 2) },
	},
}

// dbMigrations 是 SQLite 库级迁移链，按 From 升序排列。
var dbMigrations = []migration{
	{
		From: 0,
		To:   1,
		// v0 → v1：仅置 PRAGMA user_version，表结构未变。
		// 用 SQLite 原生的 user_version 而非自建 meta 表，少一张表少一处不一致。
		ApplyDB: func(db *sql.DB) error {
			_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", 1))
			return err
		},
	},
	{
		From: 1,
		To:   2,
		// v1 → v2：同样只更新版本号，不变换事件 payload（理由见 eventMigrations 的 v1→v2）。
		ApplyDB: func(db *sql.DB) error {
			_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", 2))
			return err
		},
	},
}

// adjacentSteps 返回从 from 升级到 to 所需的相邻步骤序列。
//
// 硬约束：每一步的 From 必须等于当前游标，且 To 必须等于 From+1。
// 这样「从任意旧版本升级」只依赖相邻步骤，不依赖版本组合。
func adjacentSteps(from, to int, steps []migration) ([]migration, error) {
	if from == to {
		return nil, nil
	}
	if from > to {
		// 回退旧版本需要重写数据，本机制不支持。
		return nil, fmt.Errorf("session: 不支持从 v%d 回退到 v%d", from, to)
	}

	var out []migration
	cur := from
	for cur < to {
		var next *migration
		for i := range steps {
			if steps[i].From == cur {
				next = &steps[i]
				break
			}
		}
		if next == nil {
			return nil, fmt.Errorf("session: 缺少从 v%d 出发的迁移步骤（目标 v%d）", cur, to)
		}
		if next.To != cur+1 {
			return nil, fmt.Errorf("session: 迁移步骤 v%d→v%d 不是相邻步进", next.From, next.To)
		}
		out = append(out, *next)
		cur = next.To
	}
	return out, nil
}

// probeEventSchema 从单条事件的原始 JSON 中读出格式版本号。
// 缺省（无 `v` 字段）为 0，即 v0。
func probeEventSchema(raw []byte) (int, error) {
	var probe struct {
		V int `json:"v"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return 0, err
	}
	return probe.V, nil
}

// stampSchemaVersion 在保持其余字段不变的前提下，把事件的 `v` 写为指定版本。
// 用 map 中转再序列化，输出键序由 encoding/json 固定为字典序，保证确定性。
func stampSchemaVersion(raw []byte, v int) ([]byte, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("session: 迁移事件失败（不是 JSON 对象）: %w", err)
	}
	obj["v"] = json.RawMessage(strconv.Itoa(v))
	return json.Marshal(obj)
}

// migrateEventLine 把一行事件 JSON 从它自身的版本升级到 SchemaVersion。
//
// 已是当前版本时原样返回，不做无谓的重新序列化（读路径是热路径）。
func migrateEventLine(raw []byte) ([]byte, error) {
	v, err := probeEventSchema(raw)
	if err != nil {
		return nil, fmt.Errorf("session: 解析事件格式版本失败: %w", err)
	}
	if v > SchemaVersion {
		return nil, fmt.Errorf("%w（事件为 v%d，当前支持 v%d）", ErrSchemaTooNew, v, SchemaVersion)
	}
	if v == SchemaVersion {
		return raw, nil
	}

	steps, err := adjacentSteps(v, SchemaVersion, eventMigrations)
	if err != nil {
		return nil, err
	}
	for _, st := range steps {
		if st.Apply == nil {
			continue
		}
		raw, err = st.Apply(raw)
		if err != nil {
			return nil, fmt.Errorf("session: 迁移 v%d→v%d 失败: %w", st.From, st.To, err)
		}
	}
	return raw, nil
}
