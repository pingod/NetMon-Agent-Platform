# 项目记忆（AGENTS.md）

> **已并入 NetMon-Agent-Platform**：本目录是平台内置的「IDC 上架规划与布线」模块。
> 统一入口 `/#/planning`（侧边栏「数据机房 → IDC上架布线」），API 前缀 `/api/v1/planning`。
> 独立运行/测试方式保持不变（`bash scripts/run.sh`、`pytest`、`pnpm vitest`）。
> 集成细节见 [`INTEGRATION.md`](INTEGRATION.md)。


> 本文件是**给 agent 的项目记忆**，每次在本仓库工作时自动加载。
> 人类交接文档见 [`docs/HANDOFF.md`](docs/HANDOFF.md)；全貌见 [`README.md`](README.md)。
> 最后更新：2026-10-28，v1.0 完成 3D 机房视图 + 离线交付双形态（绿色包为默认，见 §1 表底两条与 HANDOFF §3.0）。
> **提交数与会话无关，别写死在文档里** —— 想知道就 `git rev-list --count HEAD`。

---

## 0. 开工前 30 秒

```bash
cd /path/to/NetMon-Agent-Platform/planning

# 五条验收（全绿才能动代码）
cd backend && env -u PYTHONHOME -u PYTHONPATH .venv/bin/python -m pytest tests/ -p no:warnings   # 1328 passed
env -u PYTHONHOME -u PYTHONPATH .venv/bin/lint-imports                                          # 5 kept
env -u PYTHONHOME -u PYTHONPATH .venv/bin/python scripts/verify_export.py                        # 0 格差异 ★
cd ../web && ./node_modules/.bin/vitest run                                                      # 255 passed
./node_modules/.bin/tsc --noEmit                                                                 # 零错
```

**★ 导出铁线**：生成的 24 列 × 5127 行与客户原表逐格比较，必须 `123072 格，差异 0 格`。
**任何时刻它变成非 0，就是引入了真 bug —— 不接受、不"先提交后修"。**

---

## 1. 环境铁律（每条都踩过，别再踩）

| 约束 | 原因 | 正确做法 |
|---|---|---|
| **python 必须加前缀** | `PYTHONHOME` 被污染成 `/tmp/.mount_DeepseXXXX/usr/`，直接跑报 `Fatal Python error: Failed to import encodings module` | `env -u PYTHONHOME -u PYTHONPATH .venv/bin/python` |
| **别用 `pnpm vitest` / `pnpm build`** | pnpm store 只读，报 `[ERR_SQLITE_ERROR] unable to open database file` | `./node_modules/.bin/{vitest,vite,tsc}` |
| **pytest 必须在 `backend/` 下跑** | 否则 `ModuleNotFoundError: No module named 'idcbox'` | `cd backend` 再跑 |
| **多行中文 commit message 必须落文件** | `git commit -m "…<pattern>…"` 被 shell 当重定向，报「没有那个文件或目录」 | `git commit -F .scratch/commit_msg.txt` |
| **后台 uvicorn 必须用 managed background job** | `(... &)` 起的进程会被杀：端口 LISTEN 但 `ps aux` 查不到 → `curl` 拿到**旧进程**的响应 | 用 `bash` 工具的 `run_in_background` |
| **Chrome 截图每次换新 profile** | 复用 `--user-data-dir` 会截到上一次的缓存页（症状：三张图字节数完全一样） | `--user-data-dir=$(mktemp -d)` |
| **含中文的 heredoc 内联 python 会炸** | `SyntaxError: invalid character '，' (U+FF0C)` | 写 `.scratch/xxx.py` 再执行 |
| **GitHub/pypi 直连时好时坏** | HEAD 200 但 body 超时；容器出口 IP 还会被 TUNA 403 | PBS 走 npmmirror（`--python-mirror`）；pip 走阿里云（`--pip-mirror` / `PIP_INDEX_URL`）；**sha256 无条件校验** |
| **`pkill -f <pat>` 在 `bash -c` 里自杀** | pattern 字符串就在自己的命令行上，SIGTERM 掉执行它的 shell | 放进脚本文件执行，或 `ss -ltnp` 查到 pid 再 kill |

`scripts/run.sh` 起服务：`bash scripts/run.sh`（默认 8000）。深链 `#scene3d|#floor|#design|#cables|#health|#ports|#consumables|#projects|#compare|#ledger|#models|#labels`；平面图拖设备加 `?edit=1`。

---

## 2. 五条不可协商的代码规矩

1. **`domain/` 零 IO** —— 不 import 数据库/HTTP/文件系统/openpyxl/pydantic。输入输出全是 dataclass。
   想在里面 import openpyxl 的时候，说明这段该放 `infrastructure/`。
   被 `import-linter` L1 契约强制（**函数体内的延迟 import 也会被抓**，见 §6.3）。
2. **公式单点** —— `domain/cable_length/formulas.py` 是 N/O/P/Q/R/S/T 唯一实现。
   `web/src/domain/cableLength.ts` 是镜像，**必须共享同一份 `shared/golden/length_vectors.json`**。
3. **适配器收口** —— 第三方库类型只许出现在 `infrastructure/` 与 `web/src/adapters/`。
4. **`domain/units.py` 是常量唯一中心，且不含任何公式**（尤其不许出现 `0.72`，那是 R 的推导结果）。
5. **改生成逻辑后必须跑导出铁线** —— 它比任何单测都严格。

---

## 3. 核心资产速查（**改动前必读对应反解报告，别重新推导**）

### 3.1 线长七公式 → [`docs/architecture/01-线长算法反解报告.md`](docs/architecture/01-线长算法反解报告.md)

```
N = 0 if Δrow==0 else 4*Δrow+4           # Δrow = (通道,侧别) 的平面图行号差   98.6%
O = |col_a − col_b|                       # A 类：同排（同一平面图行）          93.1%
O = (CMAX−col_a) + (CMAX−col_b) + 1       # B/C 类：跨侧/跨通道（到排端往返）    B 98.0% / C 60.0%
    CMAX[403]=38, CMAX[404]=18            # 排端走线架入口列，room 级配置
P = N + O                                 # 636/636
Q = {RJ45:7, LC-LC:5, MPO:5}              # 636/636
R = excel15(P × 0.6 × 1.2)                # ★ 必须 excel15，否则 170/636 行与 Excel 存值不符
S = excel15(R + Q)                        # 636/636
T = ladder_round(S, IFS_COMPAT)           # ★ 不是档位表，见 3.2               636/636
U = 人工核定                              # ★ 不可算法复现：352 一致/103 上调/172 下调
```

**已证伪、不要重试**：`O=|柜号差|` 36.0% / `|柜号差|+1` 7.4% / `ceil(|Δcol|/5)` 30.7% / 环形绕行 A45.6% B8.0% / 排内位置索引差 45.6% / `|Δcol|+drow` 76.6%。
**柜号在两排之间是镜像的**（403 房 ch1：北侧 col38=柜1，南侧 col38=柜27，柜 1 与柜 27 面对面）→ 按柜号差算距离一定错。

### 3.2 T 阶梯：三个实现只有一个能进生产

| 实现 | 复现 | 差行 | 用途 |
|---|---|---|---|
| `IFS_COMPAT`（逐字复刻源数组公式） | **636/636** | 0 | **生产默认** |
| `CONTINUOUS`（只把 `eq 7` 放宽成 `le 7`） | 587/636 | 49（S 恒 6.44） | 体检项 F4「多买一档」 |
| `ladder_ceil_tiers` / `ladder_round_nearest` | 424/636 / 111/636 | 212 / 525 | **仅测试负样本** |

源公式 `S<=6→7, S=7→7, S<=8.5→10` 中间有个 `(6,7)` 空档：`S=6.44→10` 但 `S=7→7` —— **不是单调阶梯**。
`ladder_round(s, rungs)` 是**规则表逐条匹配、首条命中即返回**，与"档位法"**不是同一函数**。

### 3.3 长度四策略 → `domain/cable_length/policy.py`

`GEOMETRIC`（新建主路径）/ `EMPIRICAL`（经验表建议）/ `MANUAL` / `IMPORTED`（**存量迁移：保留原值、不参与偏差告警**）。
- `calc_len_t` 与 `suggest_len_m` **任何策略下都恒算**。
- `IMPORTED` 的 `approved_len_u` 恒等于原值 —— 否则 4477 行人工直填会被"改回去"，用户失去信任。
- 经验表 `rules/len_empirical_v1.json`：三级回退 + 门控 `n≥10 且纯度≥0.75`。**LOO 87.11%；对外承诺口径「覆盖 84.31%、覆盖内 94.00%」**（85.2% 是含样本回测口径，只对内披露）。

### 3.3b ★ 拖动会改掉你没选中的行（v0.12 的血泪教训）

**v0.8~v0.11 的文档里写着「改 U 位不影响线长，所以拖一格是安全的」—— 那句话是错的。**

它只看到了线长。端口号也会变：`端口号 = 组内位次 + base`，位次按
`(机柜升序, U 位升序)` 排 —— 换柜或换槽位就换位次，**同组排在它后面的
设备端口号整体前移**。

| 动作（V13 真数据，ch2 机柜1） | 端口重编号 | 线长变化 |
|---|---|---|
| 柜内 3U → 5U（同槽位） | 0 | 0 |
| 柜内 3U → 39U（第 10 槽） | **56 行 / 7 台** | 6 |
| **跨柜 机柜1 → 机柜5** | **91 行 / 7 台** | 6 |

**铁律**：
- **判据是「位次变没变」，不是「柜变没变」** —— 同柜内换个 U 也可能改 56 行。
- 任何移动入口都必须返回 `impact`（`domain/ports/gen_diff.py` +
  `application/services/impact.py`），并在界面说出来。
- `lengthChecked=false` 时 `lengthChanges=0` 是「**没算**」不是「没变」——
  界面必须说「线长未校验」，说「线长没变」就是说假话。
- 身份键**不含端口号**（否则「换口」会被报成「删一行加一行」）。
- 影响面里的设备名**不能是槽位 id**（`N02-01-S6` 现场找不到那台机器），
  要翻成 `型号 机柜N 27-28U`；有测试对整份 payload 扫描槽位 id。

### 3.3c 3D 几何的三个「不报错但画错」的坑（v1.0）

3D 的错误**几乎都是"差一点"**：柜子穿模 100mm、相机远一倍、设备高一个 U。
屏幕上不一定看得出（机房那么大，多 100mm 谁知道），但打印或换机房就对不上。

| 坑 | 症状 | 正解 |
|---|---|---|
| 按「平面图行号 × 600mm」算 z | 两排柜穿模、**通道消失**（柜深 1200 > 格距 600） | `z(通道c北侧) = c × (2×柜深 + 通道宽)`，南侧再加（柜深+通道宽） |
| 两房各按自己最左列算 x | **146 个柜坐标重复**，像"柜数翻倍" | 按房间排开 + 留一条通道宽空隙 |
| 相机用 `hypot(宽,深)` | 49 米距离，机柜只占画面中间一小块 | 包围盒 8 角**投影到相机 right/up 轴**，按水平/垂直视场角各求距离取大者 |

**柜体画法试了三种，只有第三种对**：
① 半透明实体 → 362 个透明盒叠加排序不稳，**一整片黑雾**；
② 半透明 + `EdgesGeometry` 线框 → `LineSegments` 在 **SwiftShader 下渲染成放射状尖刺**；
③ **`BackSide` 不透明盒 + 顶面亮条** → ✅ 只画内壁，像个敞开的抽屉，既无不透明排序问题又能看见设备。

> ②的失败最值得记：**它可能在有独显的真机上正常，只在软渲染下坏**。
> 所以 **3D 验收必须在软渲染环境做**（`--disable-gpu --use-gl=swiftshader`）
> —— 客户现场往往就是没独显的跳板机，否则测出来的"好"是假的。

**铁律**：
- **禁止 `devices.map(d => <mesh/>)`** —— 3000 个 Mesh = 3000 draw call，掉到 15fps。
  实测 10 个 draw call 在**无显卡**下 60fps（AC-7 只要求 30）。
- `mmToWorld = mm/1000` 是**全项目唯一的 /1000**（three 用米）。多一处就多一次差 1000 倍。
- 数据里只有毫米；柜宽 600mm 取布线表 R 列的 `0.6m/柜位`，**不是另拍一个**。
- 设备分类**只许后端给**（`svg.py::categorize()`），前端重判会"屏幕红色、纸上黄色"。
- 拖动结束那次 `pointerup` 也会触发 `click` —— 不判「位移 < 4px」，
  **每次转视角都会误跳去某个机柜**。

### 3.4 端口分配五规则 → [`docs/architecture/03-端口分配算法反解报告.md`](docs/architecture/03-端口分配算法反解报告.md)

| 规则 | 内容 | 实测 |
|---|---|---|
| C1 | 服务器端口名 → 交换机型号，64 条零歧义 | 100% |
| C2 | `端口号 = 组内位次 + base`；组 = (交换机型号, 实例名, 对端型号)，组内 `(rack asc, u asc)` | **4562/4562** |
| C3 | `slot_i ≡ (i+φ) mod 8`，`slot_of(u)=(u−3)//4` | **仅计算节点 112/112；全部节点 68.29%** |
| C4 | `φ ≡ slot_of(首节点 U)`，不落库、实时算 | 同上口径 |
| C5 | 组内机柜号严格单调不减 | 计算节点 112/112 |

**方向铁律**：`端口号 → 节点位置`（正向生成），不是反向猜。已证伪：全局序位次 65.82%，柜内起点取最大 U 后旋转 **0.00%**。

### 3.5 三条规则链 → `domain/ports/flow.py`

①柜内相位轮转 `(slot − carry%8)%8`（不轮转 82.55%）②重复槽位顺延整轮 `k*8 + 轮转序号`（21 组）③连续切块 + 末台吸收余量（`ch2 200/5→[44,44,44,44,24]`、`ch9 134/3→[44,44,46]`）。

### 3.6 意图模型 → `domain/ports/intent.py`

949 行 = racks 28 + switches 130 + devices 34 + routes 34 + chains 26 + explicitLinks 697 → 5127 行布线表。
**诚实边界**：4430/4562 = 97.11% 由规则覆盖；剩 132 行「一台一种接法」必须走 `ExplicitLink`。
**位置覆盖**：`devices` 段里 `instance` 恰为槽位 id（`N02-01-S0`）的条目覆盖该槽设备位置，但 **`node_id` 不变** —— 换 node_id 会让端口分配链整体错位，那个柜的 44 根线全接错口。

---

## 4. 口径陷阱（**这个项目最大的坑，历史上踩过 6 次**）

遇到"数字对不上"，**先问分母是什么**，别先怀疑实现。

| 同一个东西的多个口径 | 都对，但用错就错 |
|---|---|
| 设备数 | **824**（物理位置 = 通道+机柜+U位）/ **834**（型号归一化后实例）/ **836**（型号原样）/ **262**（不含 U 位） |
| 行数 | **5127**（有效数据行）/ **5113**（按 11 列组合去重）/ **5128**（`max_row` 含表头） |
| 经验表覆盖 | **84.31%**（对外承诺，含 C 类+MPO 强制人工）/ **85.2%**（含样本回测，只对内） |
| 端口相位 C3 | **112/112**（仅计算节点）/ **68.29%**（全部 174 块） |
| U 位重叠 | **13 对**（型号原样）/ **11 对**（归一化，去掉 2 对空格变体假冲突） |
| R30 模块/批次错位 | **36 行**（真实模块型号）/ **126 行**（模块列任一非空，含 `一` 这类无需采购的） |
| N 公式分母 | **636**（有 P 的行）/ **641**（有 N 值的行，体检口径 627/641=97.82%） |

**`U位` 列在布线表里出现两次**（第 5 列本端、第 11 列对端），导出时两列同名，别以为是 bug。

---

## 5. 已知的诚实边界（**不许对外宣称"全自动"的部分**）

| # | 事实 | 必须怎么做 |
|---|---|---|
| 1 | **U 核定长度不可算法复现**（352 一致/103 上调/172 下调，**下调占多数**） | `calc_len_t` 只读 + `approved_len_u` 可编辑 + 调整人/时间/原因/原值留痕。不能假设"人工只会往上加余量" |
| 2 | **模块列只覆盖 570/5113 行（11.1%）** | 汇总**必须显式标覆盖率、禁止外推**（按 37.61% 外推会少买，现场停工级事故）。模块列默认可空、**绝不强行生成** |
| 3 | **采购量不可算法复现**（实测倍数 1.124/2.314/1.560/1.250，原表"加 30% 冗余"是假的） | 需求可 100% 复现；采购量人工核定 + 两列对照 |
| 4 | **U 位重叠 13 对**（含 4 对真撞车） | `UNIQUE(rack_id,u_start)` **不能作硬约束**，降级为告警 + `device_conflict` 人工裁决 |
| 5 | **`device_instance.name` 可空**（631 个位置无名） | 绝不能当唯一键。用 `UNIQUE(rack_id,u_start)` + 系统生成 id |
| 6 | **C3/C4/C5 只对计算节点 100%** | 相位校验按节点型号开关，非计算节点只告警不阻断 |
| 7 | **源表型号记错**（`管理接入交换机-1` 自报 A 被引用写 B） | 报「模板里没有某端口」时先查 `modelConflicts`，**别说成模板问题**（修的人会去改模板把模板改坏） |

`test_golden_length.py::test_u_relation` 断言 `down>0 且 same 占比<0.60` —— 这是**守门员**：若哪天 U 真可算了它会红，提醒你重新设计。

---

## 6. 踩过的真 bug 教训（同类错误别再犯）

### 6.1 静默错误最危险
- **`_slot_id_at` 反查退回"按型号+U位找第一台"** → 批量拖动把"不存在的设备"当成真设备，返回成功但动的是用户没选的那台，**无任何报错**。修法：实例名只可能是型号或槽位 id，都不是就判找不到。
- **覆盖记录按槽位 id 暴露给前端、前端按 displayName 比对** → 「拖过一次就再也拖不动，且不报错」。
- **`GET /api/design` 读磁盘而编辑器存库里** → 「保存成功但布线表不变」。

### 6.2 计数被样本覆盖
`device_payload` 把 `**counts` 和样本混一层 → API 传 `limit=30` 时「4886 处差异」被报成「30」。
→ **计数必须独立字段**（`added_total`）、**样本与计数不同键**（`counts` 单独一层）。
**教训**：`assert count >= 5` 这种弱断言漏掉了真事故 —— 断言要写死具体数字。

### 6.3 import-linter 拦函数体内延迟 import
`from ..infrastructure import x` 写在函数体里**仍报 BROKEN** —— 契约扫的是所有 import，拦的正是假解耦。
正解：模块级注册表 `set_stock_reader()`，由组合根注入。

### 6.4 测试假设错 ≠ 实现对错
这个项目里至少 15 次「测试红了但实现是对的」：`test_prst` 用我们算的 N/O 喂 P/R/S（必须喂**源** N/O）；`excel15` 用 `isclose` 测不出差异（必须精确相等）；`InstanceInventory.count_*` 是实例数不是冲突对数；`limit=0` 是"零条"不是"不限"；`slot_of(41)==1` 不是 2；`diagnose_phase` 只列违规节点。
→ **红了先读实现，别急着改实现**。

### 6.5 两个入口必须共用同一道判据

v0.11 给**批量**路径加了名字白名单（「布局段设备的实例名只可能是型号或槽位 id」），
但**单台**路径漏了。后果：`POST /api/intent/move` 传一个不存在的名字时，
`_guess_model` 只看机柜属于哪个布局块（不看名字）→ `_slot_id_at` 退回
「按型号 + U 位找第一台」→ **成功改掉一台真机的位置**，返回 `ok=true`。
同样的动作走 `move-batch` 却被正确拒绝。

**两个入口对同一件事给出相反答案，比两个都错更难发现** —— 用户会以为是
「单台能拖、批量不能拖」。加新校验时先问：还有别的入口做同一件事吗？

### 6.6 subshell 后台进程：`$!` 不是你以为的 pid

`( cd x && ./run.sh ) &` 里 `$!` 是 **subshell** 的 pid；run.sh 的
`exec python` 发生在 subshell 的**子进程**上 —— trap 拿 `$!` 杀不掉 uvicorn，
留下一具占端口的僵尸把下次自检挡死。正解：`( cd x && exec ./run.sh ) &`
（子 shell 被 run.sh 顶替，pid 对齐）。同理，**断网模拟脚本里所有
本机 curl 必须 `--noproxy '*'`**，否则对 127.0.0.1 的探测也被死代理劫持，
误报「服务未就绪」。

### 6.7 其它高价值教训
- **`setPointerCapture` 期间兄弟组件收不到事件** → 跨柜拖拽落点用 `document.elementFromPoint` + `closest('[data-rack]')`。
- **批量拖动必须是"同一 delta"不是"各自新 U 位"** —— 8 台整体下移一格时每台新位置恰是下一台旧位置，逐台校验一台都动不了。**这是正确性问题不是性能问题**。
- **`_cross_rack_problem` 不能对所有设备套同一条约束**（ch1 网络/安全区一段布局都没有，导致核心交换机一台都换不了柜）。
- **`intent_volume` 漏统计 `devices` 段**（报 915 实际 949）。
- **`norm_u_position` 不统一区间符号**（`3—12U` 中文破折号原样返回）→ 归一结果被用作设备身份键，不收敛会把同一台报成两台。
- **★ React 闭包快照让 `onPointerUp` 拿旧 `draft`**（v0.12 查了三轮）：拖动中
  任何 `setState`（比如预演回填 `setPreview`）→ `draft` 是新对象 →
  `finishDrag` 这个 `useCallback` 重建并重新绑定；但**已经发生**的 pointerdown
  之后，松手时用的仍是按下那次渲染的 handler，闭包里 `draft` 是旧的。
  症状是**松手什么都不发生**：不发请求、不报错、notice 空着。
  修法：`onPointerUp` 走 `finishDragRef.current`；同理 `dragging` 判定读 `draggingRef`。
  **排查这类问题的第一步是先确认「handler 到底有没有执行完」**，别先怀疑网络或坐标。
- **验证脚本的落点算错会伪装成产品 bug**：用 `.rack-body-wrap svg` 的 rect
  反算 U 号，而 `uAtPointer` 用的是 `svgRef` 那个 svg —— 两者尺寸不同，
  算出的 y 落到 svg 之外 → `target=null` → `finishDrag` 直接 return。
  **「验证工具错了」和「产品错了」在表面上完全一样**，所以要先用探针确认
  中间值，再判断是谁的错。

---

## 7. 还没做的（**别自己开工，先问**）

| 项 | 状态 | 为什么现在不做 |
|---|---|---|
| **3D 机房视图** | ✅ v1.0 已做（见 §3.3c） | **但三道 gate 只过了可验证的那条**：G-3D-3（2D 稳定）✅；G-3D-1（客户付费）/G-3D-2（汇报场景）**是商业判断，代码侧无法验证**。摘掉代价小（3 个前端文件 + 1 服务 + 1 端点 + 3 依赖），见 `docs/HANDOFF.md` §5.8 |
| 3D 的 FR-7.5（连线示意） | ⬜ 不做 | 产品规划已明确排除；即使做 3D，走线示意也是最后一项 |
| **施工状态**（布了/贴了/上电了） | ⬜ v1.0 | 用户 m00001 原话「后期管理维护、自动化巡检，**现在不做，后面做的时候再说**」，归「管得住」范畴，和巡检/工单一起设计 |
| 意图分解增量更新 | ⬜ | 全量重建 824 台下 200ms 可接受，上万台才需要 |
| 多人同时编辑 | ⬜ v0.3+ | Q4：先「1 主编辑 + N 只读」 |
| ~~Docker / 绿色包~~ | ✅ v1.0 已做 | `scripts/package_green.sh` 绿色包（嵌入式 Python 3.12 双平台合并包 67MB，`自检.sh` 断网验 AC-8）+ Docker 形态②。依赖清单以 pyproject `[infra,api]` 为准（v1.0 才补上，此前是散装） |

---

## 8. 用户已拍板的五件事（`README.md` §四，别再重问）

| Q | 结论 |
|---|---|
| Q1 专用 vs 通用产品 | **先专用，但几何参数从第一天外置**（省 8~10 人日，规避 30% 返工） |
| Q2 导入历史 vs 新建优先 | **导入优先**（客户手上有 V13；导入器顺带产出体检报告，是建立信任的第一步） |
| Q3 Docker vs 绿色包 | **双形态，绿色包为交付默认** |
| Q4 多人编辑 | **1 主编辑 + N 只读** |
| Q5 3D | **v0.1~v0.3 不做，v1.0 按三道 gate 评估** |

---

## 9. 目录速查

```
backend/idcbox/
  domain/          ← L1 纯函数，零 IO           ★ 核心资产都在这
    cable_length/  ← 七公式/ladder四策略/engine
    ports/         ← intent/flow/generate/canon/moveu/gen_diff/portspec/ledger
    geometry/      ← 平面图解析（362 柜位）
    exporting/     ← contract25/labels/consumables/svg/stockdiff
    validation/    ← 体检 11 规则
  application/     ← L2 services + 8 个 Protocol（含 scene.py = 3D 场景）
  infrastructure/  ← L0 db(36 表)/xlsx/config
  api/main.py      ← L3 40 个端点 + SPA 托管
  cli.py           ← 17 个子命令
web/src/           ← React 19 + TS + Vite，十二页签（含 3D，three 懒加载）
projects/v13/      ← intent_full.json（949 行意图）
rules/             ← len_empirical_v1.json（经验表）
信息港部署上架/     ← 客户原始交付物（只读，勿改）
docs/              ← 16.4k 行文档（含 .bak），见 docs/HANDOFF.md §四
.scratch/          ← 798MB 反解临时脚本，**不入 git**
```

**数据重建**：`data/idcbox.db` 不入 git。要重建用 `idcbox project-save`（存意图）或 `idcbox decompose`（落规范表 20132 行）。

---

## 10. 干活节奏建议

- 改完跑 §0 五条验收，全绿再提交。
- **一次提交只做一件事**，commit message 写清「为什么」而不是「改了什么」（历史提交可作范例：`git log --oneline`）。
- 文档同步不是可选项：新能力要进 `README.md` 对应章节 + 更新 §二 的用例数，否则下一个接手的人会读到过时数字（**已发生过**：README 里 `915` 应是 `949`，本次交接才修）。
- 遇到本文件与代码不一致时：**以代码 + 测试为准**，然后回来修本文件。
