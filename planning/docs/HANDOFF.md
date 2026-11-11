# 交接文档（HANDOFF）

> 写于 2026-10-25，交接时仓库状态：**v0.12 + v1.0 的 3D 已完成，工作树干净**。
> 版本提交见 `git log --oneline`。**3D 的三道 gate 只过了可验证的那条**
> —— 见 §5.8，别以为它已经"通过评审"了。
> 这份文档的目标：**换一个新会话/新的人，读完这一份就能接着干活，不需要读十几份共 16.4k 行的文档。**
> 需要更多背景时，按第四节「文档地图」逐层展开。

---

## 一、三十秒了解这个项目

**做什么**：把 IDC 实施前那份 5127 行的《设备上架规划》Excel，变成「填约 30 行接线意图 → 自动生成 5127 行布线表 → 原样导出 25 列 → 顺手告诉你表里哪儿错了」。

**为什么值得做**：那份 Excel 里有一个隐性事实 —— **5113 条有效连线里只有 636 行（12.4%）真的走了公式链，剩下 4477 行（87.6%）连 N/O/P 都没填，长度是人凭经验直接敲的**。所以竞争点不是"把 Excel 搬到网页"，而是三件 Excel 做不到的事：几何自动化、布线表生成、数据体检。

**当前形态**：Python 后端（域层纯函数 + FastAPI + SQLite）+ React 前端（十一页签）+ CLI（17 个子命令）。**完全离线单机跑**，`bash scripts/run.sh` 一条命令启动。

**规模**（2026-10-25 实测，以 `wc -l` / `find | wc -l` 为准，别背数字）：
后端 79 个 Python 文件 + 54 个测试文件；前端 33 个 TS/TSX + 4008 行 CSS；
`docs/` 16.4k 行 Markdown（含 `.bak/` 历史副本，去掉后 11.3k）。

> 历史文档里写过「22k 行文档」—— 那个数字把 `.bak/` 副本与重复统计算进去了，
> 别引用。**口径错了比数字错了更难发现**（§九第 4 条）。

---

## 二、交接时的验收状态（全部实测，可复现）

| 项 | 结果 | 怎么复现 |
|---|---|---|
| 后端用例 | **1328 passed**（golden 293 / unit 769 / integration 263 / perf 3） | `cd backend && env -u PYTHONHOME -u PYTHONPATH .venv/bin/python -m pytest tests/ -p no:warnings` |
| 前端用例 | **255 passed**（8 文件） | `cd web && ./node_modules/.bin/vitest run` |
| 分层契约 | **5 kept, 0 broken** | `cd backend && env -u PYTHONHOME -u PYTHONPATH .venv/bin/lint-imports` |
| **导出保真铁线** | **123072 格，差异 0 格** | `cd backend && env -u PYTHONHOME -u PYTHONPATH .venv/bin/python scripts/verify_export.py` |
| 生成行位对齐 | **5127 / 5127**（多出 0、缺失 0） | 见 golden `test_golden_generate.py` |
| 前端构建 | `tsc --noEmit` 零错、`vite build` 通过 | `cd web && ./node_modules/.bin/tsc --noEmit && ./node_modules/.bin/vite build` |
| 导出铁线含义 | 生成的 24 列 × 5127 行与客户原表**逐格完全相同**（含 2 个同名 `U位` 列、`待定`/`一`/`设备自带` 等 sentinel 文本） | — |

> ⚠️ **「导出铁线」是这个项目最重要的一条回归**。任何时候它从「0 格差异」变成非 0，就是引入了真 bug，不要接受、不要"先提交后修"。

---

## 三、怎么跑起来（含本机特有的坑）

### 3.0 交付形态（v1.0：双形态，绿色包为默认 —— Q3 拍板）

```bash
bash scripts/package_green.sh           # 开发机构建（联网一次）→ build/*.tar.gz 约 67MB
docker compose up -d --build            # 形态②备选（客户允许 Docker 时）
```

现场断网部署（AC-8）：`tar xzf … && cd … && ./run.sh`（Windows 双击 `run.bat`），
验收动作 = 包内 `./自检.sh`（解释器/依赖/导出铁线/服务探测，全程零网络）。

**五条别踩的铁律**（每条都在 `package_green.sh` 注释里展开）：
1. **venv 不可打包** —— 绝对路径 + 平台绑定必坏。包内 = standalone 解释器
   （PBS 3.12.11，tag+sha256 钉死）+ `--target` 双平台合并 site-packages +
   源码走 `PYTHONPATH`（现场零 pip）。
2. **镜像只是换腿，哈希才是守门人** —— GitHub 直连/gh-proxy/TUNA 在本机
   全被节流或 404，最终 npmmirror；但缓存命中也必须过 sha256。
3. **依赖清单以 pyproject `[infra,api]` 为准**（v1.0 才补上的组；此前
   fastapi/uvicorn/sqlalchemy 是开发机散装、清单不可复现）。
4. **`run.bat` 必须 CRLF**，构建期断言 —— 该问题在 Linux 上永远测不出。
5. **模拟断网自检的本机探测要 `--noproxy '*'`**，否则 curl 跟着死代理走，
   永远探测「未就绪」（真踩过，别怀疑产品）。

### 3.1 一条命令启动 Web

```bash
cd /path/to/NetMon-Agent-Platform/planning
bash scripts/run.sh                      # 默认 http://127.0.0.1:8000
IDCBOX_PORT=8123 bash scripts/run.sh     # 换端口
IDCBOX_SOURCE=/path/to/V13.xlsx bash scripts/run.sh   # 换源数据
IDCBOX_INTENT=projects/v13/intent.json bash scripts/run.sh  # 换成精简意图
```

**深链**：`#scene3d` / `#floor` / `#design` / `#cables` / `#health` / `#ports` / `#consumables` / `#projects` / `#compare` / `#ledger` / `#models` / `#labels`。平面图上拖设备要加 `?edit=1`。

### 3.2 必须知道的四个环境坑（本机实测，踩过不止一次）

1. **`PYTHONHOME` 被污染**成 `/tmp/.mount_DeepseXXXX/usr/`。直接 `python3` 会报
   `Fatal Python error: Failed to import encodings module`。
   → **所有 python 调用都必须加前缀 `env -u PYTHONHOME -u PYTHONPATH`**。

2. **`pnpm vitest` / `pnpm build` 会失败**（`[ERR_SQLITE_ERROR] unable to open database file`，pnpm 的 store 目录只读）。
   → 一律用 `./node_modules/.bin/vitest run` / `./node_modules/.bin/vite build` / `./node_modules/.bin/tsc --noEmit`。

3. **从仓库根跑 pytest 会 `ModuleNotFoundError: No module named 'idcbox'`** → 必须 `cd backend` 再跑。

4. **多行中文 commit message 必须写文件**：`git commit -m "多行…<pattern>…"` 会被 shell 当重定向解析（报「没有那个文件或目录」「权限不够」「未预期的记号 newline」）。
   → 写 `.scratch/commit_msg.txt` 再 `git commit -F .scratch/commit_msg.txt`。

**另外两个**：
- 后台 uvicorn **必须用 managed background job**。用 `(... &)` 起的进程会被杀：端口 LISTEN 但 `ps aux` 查不到、`curl` 拿到旧代码的响应。失败症状极隐蔽（前端缺字段但你以为是前端 bug）。
- Chrome headless 截图：`/opt/google/chrome/chrome --headless --disable-gpu --no-sandbox --hide-scrollbars --user-data-dir=$(mktemp -d) --window-size=1680,1000 --virtual-time-budget=20000 --screenshot=<绝对路径> "http://127.0.0.1:<port>/#<tab>"`。**每次必须换新 user-data-dir**，复用会截到上一次的缓存页面（三张图字节数完全一样就是这个原因）。

### 3.3 数据从哪来

- 源数据（客户交付物，只读）：`信息港部署上架/` 下 13 个文件。主数据源是
  `V13(内部)甘肃信息港-设备上架规划-20260714.xlsx`（12 个 sheet，`布线表` 5127 行）。
- 意图（我们反解出来的）：`projects/v13/intent_full.json`（全量，949 行意图，用于生成 5127 行）
  与 `projects/v13/intent.json`（精简，不含显式连线）。
- 数据库：`data/idcbox.db`（SQLite，**不入 git**，用 `idcbox project-save` / `decompose` 重建）。
- 经验表：`rules/len_empirical_v1.json`（1411 行，线长经验规则，见 §5.3）。

---

## 四、文档地图（按「想知道什么」选，别通读）

| 想知道 | 读 |
|---|---|
| 全貌 + 每个版本做了什么 | [README.md](../README.md)（1213 行，§一~§十六，**最适合从头读的一份**） |
| 产品定位、路线图、定价、风险 | [01-产品规划.md](product/01-产品规划.md)（905 行） |
| 功能需求（FR/NFR/AC）、验收标准 | [01-需求规格说明书.md](requirements/01-需求规格说明书.md)（933 行） |
| 源数据到底有哪些脏、规模多少 | [00-源数据实测基线.md](architecture/00-源数据实测基线.md)（432 行） |
| **线长七公式怎么反解出来的** | [01-线长算法反解报告.md](architecture/01-线长算法反解报告.md)（764 行，**核心资产**） |
| 系统架构、数据库 DDL、ADR | [02-系统架构设计.md](architecture/02-系统架构设计.md)（1327 行） |
| **端口分配五条规则怎么反解的** | [03-端口分配算法反解报告.md](architecture/03-端口分配算法反解报告.md)（412 行，**核心资产**） |
| 开源 DCIM 调研（为什么自研） | [01-opensource-dcim.md](research/01-opensource-dcim.md)（1075 行） |
| 2D/3D/表格/Excel 选型依据 | [02-visualization-stack.md](research/02-visualization-stack.md)（1433 行） |
| 布线算法 / 校验规则 / 导入导出幂等 | [03-cabling-algorithms.md](research/03-cabling-algorithms.md)（1586 行） |
| 商用 DCIM 功能矩阵 | [04-commercial-products.md](research/04-commercial-products.md)（432 行） |
| V13 体检报告（真实输出） | [audit_v13.md](../backend/reports/audit_v13.md)（354 行）、[reports/audit_v13.md](../reports/audit_v13.md) |

---

## 五、核心资产：反解出来的规则（**这是整个项目的护城河，改动前务必读对应文档**）

### 5.1 线长七公式（`backend/idcbox/domain/cable_length/formulas.py`，唯一实现处）

| 列 | 公式 | 实测命中 |
|---|---|---|
| **N** 跨机柜 | `N = 0 if Δrow==0 else 4*Δrow+4`，Δrow = 两设备 (通道,侧别) 的**平面图行号差** | 627/636 = 98.6% |
| **O** 间隔机柜 | A 类（同排）`O = abs(col_a − col_b)`；B/C 类（跨侧/跨通道）`O = (CMAX − col_a) + (CMAX − col_b) + 1`，`CMAX[403]=38 / CMAX[404]=18` | A 93.1% / B 98.0% / C 60.0%，合计 91.8% |
| **P** 机柜数量 | `P = N + O` | 636/636 |
| **Q** 机内线缆 | 查表：`RJ45→7`、`LC-LC→5`、`MPO→5` | 636/636 |
| **R** 柜间线缆 | `R = P × 0.6 × 1.2` = `P × 0.72`（**必须走 `excel15` 收敛到 15 位有效数字**，否则 170/636 行与 Excel 存值不符） | 636/636 |
| **S** 线缆2 | `S = R + Q`（同样走 `excel15`） | 636/636 |
| **T** 线缆长度1 | 阶梯规则表（**不是**档位表），见 5.2 | 636/636 |
| **U** 线缆长度（核定） | **人工值，不可算法复现**：T→U 一致 352 / 上调 103 / 下调 172 | — |

**O 的物理含义**（关键洞察）：走线架只架在**每排端部**，不在通道中间。同排面对面直接横穿 `O=|Δcol|`；跨侧/跨通道必须先走到本排端部入口、再从对端排端折返，所以多了往返距离。这解释了源表里所有看似莫名其妙的 `+5/+11/+13/+17` 偏移。

**已被证伪、不要再试的模型**（全部实测失败）：`O=|柜号差|` 36.0% / `|柜号差|+1` 7.4% / `ceil(|Δcol|/5)` 30.7% / 环形绕行(A 45.6%, B 8.0%) / 排内位置索引差 45.6% / `|Δcol|+drow` 76.6%。**柜号在两排之间是镜像的**（403 房 ch1 北侧 col38=柜1、南侧 col38=柜27，即柜 1 与柜 27 面对面），按柜号差算距离一定错。

### 5.2 T 列阶梯（`domain/cable_length/ladder.py`）—— 三个实现，只有一个能用

源单元格是数组公式 `=_xlfn.IFS(S2<=5,5, S2<=6,7, S2=7,7, S2<=8.5,10, …)`。**它不是单调阶梯**：`S=6.44→10` 但 `S=7→7`，因为 `le 6` 之后紧跟 `eq 7`，中间 `(6,7)` 是个空档，落进去会提前跳到 10。

| 实现 | 复现 T | 差行 |
|---|---|---|
| **`IFS_COMPAT`（逐字复刻）** | **636/636 = 100%** | 0 ← **生产默认** |
| `CONTINUOUS`（只把 `eq 7` 放宽成 `le 7`） | 587/636 | **49**（失败行 S 恒为 6.44） |
| `ladder_ceil_tiers`（采购档位向上取整） | 424/636 | 212 ← 仅测试负样本 |
| `ladder_round_nearest`（档位四舍五入） | 111/636 | 525 ← 仅测试负样本 |

**命名铁律**：`ladder_round(s, rungs)` 是**规则表逐条匹配、首条命中即返回**；它和上面两个「档位法」函数**不是同一个东西**，档位法**不得进生产路径**。

### 5.3 长度四策略（`domain/cable_length/policy.py`）—— 存量迁移的唯一出路

`LengthPolicy` 四值：`GEOMETRIC`（公式链，新建项目主路径）/ `EMPIRICAL`（经验表建议值）/ `MANUAL`（人工填）/ `IMPORTED`（**存量迁移：保留原值，不参与偏差告警**）。

- `calc_len_t` 与 `suggest_len_m` **任何策略下都恒算**（否则失去对比基准）。
- `IMPORTED` 行的 `approved_len_u` 恒等于原值，重算不覆盖 —— 否则 4477 行纯人工直填的长度会被我们"改回去"，用户会失去对系统的信任。
- 经验表（`rules/len_empirical_v1.json`）三级回退 `(类,线缆,path) → (类,线缆) → (线缆)`，门控 `n≥10 且 纯度≥0.75`。**LOO 准确率 87.11%；对客户承诺口径是「覆盖 84.31%、覆盖内 94.00%」**（85.2% 是含样本自身的回测口径，只能对内披露）。

### 5.4 端口分配五条规则（`domain/ports/`，100% 复现 4562 行）

| 规则 | 内容 | 实测 |
|---|---|---|
| **C1** | 服务器端口名 → 交换机型号，64 条映射**零歧义** | 100% |
| **C2** | `端口号 = 组内位次 + base`；组 = (交换机型号, 实例名, 对端型号)；组内按 `(rack asc, u asc)` 排序 | 4562/4562 |
| **C3** | `slot_i ≡ (i + φ) mod 8`，`slot_of(u) = (u−3)//4` | **计算节点 112/112；全部节点只有 68.29%** |
| **C4** | `φ ≡ slot_of(首节点 U 位)`，**不落库、实时算** | 112/112（同上口径） |
| **C5** | 组内机柜号严格单调不减 | 计算节点 112/112 |

> ⚠️ **C3/C4/C5 只在「计算节点」口径是 100%**。非计算节点（网络功能服务器一个 U 位塞两台、备份一体机不守 8 槽位规律）会不满足 —— **相位校验器必须按节点型号开关，对非计算节点只告警不阻断**。对外绝不能说"100%"。

**方向铁律**：正确方向是 `端口号 → 节点位置`（正向生成），**不是** `节点位置 → 端口号`（反向猜）。已证伪：端口号 = 节点全局序位次 65.82%；柜内起点取最大 U 后旋转 **0.00%**。

### 5.5 三条规则链（`domain/ports/flow.py`，把意图变成端口分配）

1. **柜内相位轮转**：通道内柜号升序，柜内按 `(slot − carry%8)%8` 升序，`carry` = 前序机柜节点数累计。不做轮转只有 82.55%。
2. **重复槽位顺延整轮**：同柜同 slot 第 k 个的排序键 = `k*8 + 轮转序号`。全表 21 组重复槽位。
3. **连续切块 + 末台吸收余量**：前 K−1 台各吃 `capacity`（= 链中第一台的最大端口号），末台吃剩余全部、可超容量。实测 ch2 200/5→`[44,44,44,44,24]`、ch9 134/3→`[44,44,46]`。

### 5.6 意图模型（`domain/ports/intent.py`）

949 行意图 = `racks` 28（布局块）+ `switches` 130 + `devices` 34（特殊机型）+ `routes` 34 + `chains` 26（交换机组）+ `explicitLinks` 697 → 生成 5127 行布线表。

**诚实边界**：4430/4562 = 97.11% 由规则覆盖，剩下 132 行是「一台一种接法」，必须走 `ExplicitLink`（697 条显式连线）。这不是缺陷，是产品必须告诉用户的事实。

**位置覆盖机制**（拖拽的基础）：`devices` 段里 `instance` 恰为槽位 id（`N02-01-S0`）的条目，会覆盖该槽设备的 U 位/机柜，**但 `node_id` 保持不变** —— 换 node_id 就不再是交换机组第 N 台，端口分配链整体错位，那个机柜的 44 根线会全接到错误端口上。

---

### 5.7 ★ 拖动会改掉你没选中的行 —— 端口重编号（v0.12）

**这是 v0.8~v0.11 文档里那句「改 U 位不影响线长所以安全」的漏网之鱼。**

端口分配规则 C2 是 `端口号 = 组内位次 + base`，组内位次按
`(机柜升序, U 位升序)` 排（§5.4）。设备一换柜或换到不同槽位，
它在组里的位次就变了 —— **同一组里排在它后面的设备端口号整体前移**。

实测（V13 真数据，通道 2 机柜 1 的 `N02-01-S0`）：

| 动作 | 端口重编号 | 位置变化 | 线长变化 |
|---|---|---|---|
| 柜内 3U → 5U（同槽位） | 0 | 7 | 0 |
| 柜内 3U → 35U（同轮第 8 槽） | 0 | 7 | 0 |
| **柜内 3U → 39U（第 10 槽）** | **56 行 / 7 台** | 7 | 6 |
| **跨柜 机柜1 → 机柜5** | **91 行 / 7 台** | 7 | 6 |

> 「同柜内挪就安全」也是错觉 —— 换个 U 就可能换位次。
> 正确的判据只有一条：**位次变没变**，不是「柜变没变」。

**危害**：操作电脑的人和去插线的人不是同一个人。前者看到「已移到 5 号柜」
以为只动一台，后者按旧表插线全插错口，而界面上没有任何痕迹。

**做法**（v0.12）：`move` / `move-batch` 响应带 `impact`；
另有 `POST /api/intent/impact` 干跑（不改 doc）。三条口径必须分开说：

| 类别 | 含义 | 处置 |
|---|---|---|
| `portChanges` | 同一根线换了口 | **重打标签** |
| `lengthChanges` | 线长档位变了 | 改采购/施工 |
| `added` / `removed` | 接线拓扑变了 | **重新看设计** |

`lengthChecked=false` 时 `lengthChanges=0` 的含义是「**没算**」不是「没变」——
界面必须说「线长未校验」，不能说「线长没变」。

**身份键不含端口号**是关键：要检出的正是「端口号变了」。若端口号进键，
`10GE1/0/1` 消失 + `10GE1/0/33` 新增会被报成「删 1 行加 1 行」——
那是改设计，而实际是同一根线换了口（重打标签）。

**性能**：`deep=true`（含线长重算）约 90ms，`deep=false` 约 55ms。
拖动过程中用后者，松手前最后一次用前者。

**设备名不能是内部 id**：横幅曾显示 `N02-01-S6` —— 机器上没有任何东西
写着它，现场找不到那台机器。已翻成 `计算节点A（ARM） 机柜1 27-28U`。
同判据在 `labelsvc.py` 也有一份（防槽位号漏进贴纸）；有测试对整份 payload
做正则扫描，出现槽位 id 就红。

### 5.8 3D 机房视图（v1.0）—— 三道 gate 只过了一条

**先说结论**：3D 做出来了、能跑、帧率是要求的 2 倍，但**产品规划里的三道
gate 只通过了可验证的那一条**。别把「能运行」当成「已通过评审」。

| Gate | 内容 | 状态 |
|---|---|---|
| G-3D-3 | 2D 与表格已稳定 | ✅ 可验证且已通过（五条验收连续全绿） |
| G-3D-1 | 有客户为 3D 单独付费 ≥15% 合同额 | ❓ **商业判断，代码侧无法验证** |
| G-3D-2 | 汇报场景被验证 ≥2 次 | ❓ **同上** |

若 G-3D-1/2 不过，摘掉 3D 的代价很小：删 3 个前端文件 + 1 个后端服务 +
1 个端点 + 3 个依赖。**对其他功能零影响** —— three 是懒加载的独立 chunk
（529KB，gzip 135KB），不进首屏。

**渲染账（实测 10 个 draw call，软渲染 60fps）**：

| 物件 | 数量 | 方式 | draw call |
|---|---|---|---|
| 机柜 | 362 | 1 个 InstancedMesh | 1 |
| 柜顶亮条 | 362 | 1 个 InstancedMesh | 1 |
| 设备 | 834 | 按 U 数分 6 组 | 6 |
| 地板 | 2 | 2 个 Mesh | 2 |

**帧率 60.26fps**，且是 `--disable-gpu --use-gl=swiftshader`（**完全无显卡**）
下测的。AC-7 要求 30fps。这条比「有独显跑 120fps」有价值 ——
**客户现场往往是没独显的跳板机**。

**★ 3D 与 2D 共用一个几何真值**：

| 事实 | 唯一来源 |
|---|---|
| 柜宽 600mm | 布线表 R 列 `0.6m/柜位`（线长公式里那个 0.6） |
| 42U = 1866.9mm | EIA-310：1U = 44.45mm |
| 设备分类 | 后端 `svg.py::categorize()`，**3D 只消费不重判** |

分类必须后端给：「核心」与「交换机」的判定顺序是硬约束，
前端复制一份迟早会歪 → **屏幕上红色、纸上黄色**（2D 那边踩过）。

单位口径：数据里**只有毫米**，`mmToWorld = mm/1000` 是**全项目唯一的 /1000**。

**★ 三个「不报错但画错」的几何坑（都已修，别再犯）**：

1. **两排柜穿模**：按「平面图行号 × 600mm」算 z，但平面图一行是**柜位格距**、
   柜深是 **1200mm**。北侧（行3）与南侧（行4）只隔 600mm，两个 1200mm 深的
   柜互相穿进去 → 3D 里是一整块实心体、通道消失。
   正解：`z(通道c北侧) = c × (2×柜深 + 通道宽)`，南侧再 +（柜深+通道宽）。
2. **两房间完全重叠**：403 用列 23..38、404 用列 3..18，两房列号各不同。
   都按「减本房最左列」算 x → 两房柜坐标完全相同（**实测 146 个重复**），
   表现为「一个房间、柜数翻倍」。正解：按房间排开 + 留通道宽空隙。
3. **相机算错**：用 `hypot(宽,深)` 当画面尺寸 → 49 米距离，机柜只占中间一小块。
   机房是扁而长的（20400×2000×25200mm），俯视时深度大部分压进屏幕纵轴。
   正解：包围盒 8 角**投影到相机 right/up 轴**，分别按水平/垂直视场角求距离取大者。

**★ 柜体的画法（试了三种）**：

| 方案 | 结果 |
|---|---|
| 半透明实体 | 362 个透明盒子叠加，排序不稳 → **一整片黑雾** |
| 半透明 + `EdgesGeometry` 线框 | 线框在 `InstancedMesh` 里是 `LineSegments`，**SwiftShader 下渲染成放射状尖刺** |
| **`BackSide` 不透明盒 + 顶面亮条** | ✅ 采用 |

第二种的失败最值得记：**它可能在有独显的真机上正常，只在软渲染下坏**。
所以 3D 的验收必须在软渲染环境做 —— 否则测出来的"好"是假的。

`BackSide` 原理：只画内壁 → 正面剔除、视线穿到柜内设备，但侧壁/底面内表面
仍参与深度测试 → 每个柜像个**敞开的抽屉**。既无不透明排序问题，又能看见设备。
顶面亮条必须 `FrontSide`（跟柜体相反），否则翻到柜底、柜顶一片空。

**★ 禁止 `devices.map(d => <mesh/>)`** —— 3000 个 Mesh = 3000 draw call，
掉到 15fps 以下。这是**代码规范级**约束，不是优化建议。

**交互的坑**：拖动结束那次 `pointerup` 也会触发 `click` ——
不判「按下到抬起位移 < 4px」，**每次转视角都会误跳去某个机柜**。

## 六、代码地图（78 个 Python 文件 + 29 个 TS/TSX）

### 6.1 三层分层（`import-linter` 强制，5 条契约）

```
api/          ← L3：main.py（1611 行，38 个端点 = 27 GET + 11 写）+ schemas/ + SPA 托管
application/  ← L2：services/（views、design、drawing、labelsvc、modeladmin、portledger、projects、compare、pipeline）+ ports.py（8 个 Protocol）
domain/       ← L1：零第三方依赖、零 IO、纯函数
infrastructure/ ← L0：唯一允许出现第三方库类型的地方
```

**三条硬规矩**：①`domain/` 不得 import 数据库/HTTP/文件系统/Excel 库 ②`formulas.py` 是七公式唯一实现处 ③第三方库类型只许出现在 `infrastructure/` 与 `web/src/adapters/`。

### 6.2 关键文件（改之前先读这些）

| 文件 | 行数 | 职责 |
|---|---|---|
| `domain/units.py` | — | 常量唯一中心（**不含任何公式，尤其不含 0.72**） |
| `domain/cable_length/formulas.py` | 231 | 七公式唯一实现 |
| `domain/cable_length/ladder.py` | — | 三个 T 实现（只 `IFS_COMPAT` 进生产） |
| `domain/cable_length/engine.py` | 376 | `compute_link` / `compute_batch` / `compute_incremental` |
| `domain/cable_length/policy.py` | — | 四策略 + `resolve_length` |
| `domain/geometry/floor.py` | — | 平面图解析（362 柜位，403 room col23-38 / 404 room col3-18） |
| `domain/ports/intent.py` | 757 | 意图模型 + `expand_placements` + `placement_moves` |
| `domain/ports/flow.py` | 541 | 三条规则链（端口分配） |
| `domain/ports/generate.py` | 563 | 意图 → 连线 |
| `domain/ports/canon.py` | 909 | 意图 → 10 张规范表平面记录 |
| `domain/ports/moveu.py` | 1181 | 拖拽（单台/批量/跨柜） |
| `domain/ports/gen_diff.py` | 400 | **两次生成结果的差异**（端口重编号必须被检出） |
| `application/services/scene.py` | 290 | 3D 场景数据（毫米坐标，房间/机柜/834 设备） |
| `web/src/domain/scene3d.ts` | 260 | 3D 坐标换算（纯函数，唯一 /1000 处） |
| `web/src/views/Scene3D.tsx` | 470 | WebGL 渲染（InstancedMesh，禁一设备一 Mesh） |
| `application/services/impact.py` | 420 | 拖动影响面（`preview_move` / `impact_payload`） |
| `domain/ports/portspec.py` | 747 | 26 型号端口规格（来自采购清单「配置」列） |
| `domain/ports/ledger.py` | 439 | 设备台账 / 端口台账 |
| `domain/validation/rules.py` | — | 体检 11 规则 + 健康度 |
| `domain/exporting/contract25.py` | — | 25 列导出契约（公式从规则表反推生成） |
| `domain/exporting/labels.py` | — | 线缆标签三段式 |
| `domain/exporting/consumables.py` | — | 耗材 + 模块需求 |
| `domain/exporting/stockdiff.py` | 884 | 版本对比（含设备级聚合） |
| `domain/exporting/svg.py` | — | A3 施工图（`<pattern>` 画 U 网格） |
| `infrastructure/db/schema.sql` | 619 | 36 张表 |
| `infrastructure/db/canon_store.py` | 806 | 落库（确定性 ID，不用 UUID） |
| `cli.py` | 1543 | 17 个子命令 |

### 6.3 前端（`web/src/`，27 文件）

- `domain/cableLength.ts` — 公式镜像，**必须与后端共享同一份 golden 向量**（`shared/golden/length_vectors.json`）
- `store/appStore.ts`（单 zustand store + 纯 selector）、`store/useIntentDraft.ts`（编辑器草稿）、`store/dragStore.ts`（跨组件拖拽）
- `views/` 14 个页面组件，十一页签
- **不引 UI 框架**（MUI/AntD/shadcn）：界面只有 4 个 tab + SVG + AG Grid + 卡片，引 200KB 组件库换来的只是按钮好看，却带来主题覆盖/升级/离线打包三份负担
- **2D 用纯 SVG 自绘**：因为要导出 SVG 施工图（canvas 只能 `toDataURL` 出位图）。42U 刻度必须用 1 个 `<pattern>` 而非 42 条 `<line>`

---

## 七、已知的坑与诚实边界（**交接时最容易被忽略、也最容易出事的部分**）

| # | 事实 | 后果 |
|---|---|---|
| 1 | **U（核定长度）不可算法复现** —— 352 一致 / 103 上调 / 172 下调，**下调还占多数** | 系统不能假设"人工只会往上加余量"。必须做 `calc_len_t`（只读）+ `approved_len_u`（可编辑）+ 调整人/时间/原因/原值留痕 |
| 2 | **模块列只覆盖 570/5113 行（11.1%）** | 模块汇总必须**显式标注覆盖率、禁止外推**。按 37.61% 外推全量采购量会少买模块，是现场停工级事故。模块列默认可空、**绝不强行生成** |
| 3 | **采购量不可算法复现** —— 实测倍数 1.124 / 2.314 / 1.560 / 1.250（原表自称"增加 30% 冗余"是假的） | 需求可 100% 复现，采购量必须人工核定 + 两列对照，不得声称"自动算采购量" |
| 4 | **13 对 U 位区间重叠**（归一化后 11 对），其中 4 对是真撞车 | `UNIQUE(rack_id, u_start)` **不能作硬约束**（会拒绝真实数据），降级为导入告警 + `device_conflict` 表人工裁决 |
| 5 | **824 个物理位置 / 834 归一化实例 / 836 原样实例** 三个口径都对 | `device_instance.name` **可空**（631 个位置没有实例名），绝不能当唯一键 |
| 6 | **两套设备命名**（型号名 vs 实例名） | 型号是采购维度、业务角色是运营维度，两者都要建模；`NameAlias` 表要区分 model/instance/cross_system 三种 scope |
| 7 | **源表型号记错**（`管理接入交换机-1` 自报 `业务接入交换机1`、被引用时写 `带外交换机1`） | 报「模板里没有 25GE1/0/45」的根因是源数据型号错，不是模板错。**修的人会去改模板把模板改坏** → 必须把因果关系说出来 |
| 8 | **C3/C4/C5 只对计算节点 100%** | 相位校验器必须按节点型号开关 |
| 9 | **双向人工调整**：`test_golden_length.py::test_u_relation` 断言 `down>0 且 same 占比<0.60` | 这条断言是**守门员**：如果哪天 U 真的可算了，这条测试会红，提醒你重新设计 |
| 10 | **拖动会改掉用户没选中的行**（端口重编号 91 行，见 §5.7） | 任何移动接口都必须返回 `impact` 并在界面说出来；**不能因为「同柜」就跳过** |
| 11 | **单台与批量两个入口必须共用同一道判据**（v0.12 修） | v0.11 只给批量加了名字白名单，单台漏了 → 「拖不存在的设备」在单台上**成功挪动了一台真机**，批量上被拒。两入口给相反答案比都错更难发现 |

---

## 八、还没做的事（按优先级 + 为什么现在不做）

| # | 项 | 状态 | 说明 |
|---|---|---|---|
| 1 | **3D 机房视图** | ✅ v1.0 已做，但 gate 见 §5.8 | 用户 Q5 拍板。**必须先过三道 gate**：①2D 已满足现场施工 ②有客户明确为 3D 付费 ③`three`+`InstancedMesh` 在 3000 设备下实测帧率达标。技术方案已在 `docs/research/02-visualization-stack.md` 定版（three 0.186.1 + r3f 9.8.2 + InstancedMesh，全场景 draw call ≈12） |
| 2 | **施工状态**（布了/贴了/上电了） | ⬜ v1.0 | 属于「管得住」范畴，**用户明确说和巡检、工单一起设计**（原始需求 m00001：「后期的管理维护，自动化巡检等等，现在不做，后面做的时候再说」） |
| 3 | **意图分解的增量更新** | ⬜ | 现在是全量重建（先删后插 2 万行），824 台设备规模下 200ms 可接受；上万台才需要增量 diff |
| 4 | **多人同时编辑** | ⬜ v0.3+ | Q4 拍板：先按「1 主编辑 + N 只读」设计 |
| 5 | **Docker / 绿色包** | ⬜ | Q3 拍板：双形态，绿色包为交付默认（现场跳板机通常没有 Docker）。`scripts/run.sh` 已是绿色包雏形 |
| 6 | `web/src/adapters`、`workers`、`components` 三个目录 | 空 | 预留（Worker 池做 5113 行批量重算；3D 用 adapters） |
| 7 | `.scratch/` 798MB / 285 个 .py（共 422 个条目） | 不入 git | 反解过程的临时脚本。**这里才是"每个数字都可复现"的实物**，但被 `.gitignore` 了 —— 如需长期保留，挑出关键的移进 `backend/scripts/` |

---

## 九、给接手者的五条建议

1. **先跑一遍 §二 的五条验收命令**，确认基线是绿的再动代码。任何一条红了先修，不要继续叠加。
2. **改动线长/端口相关代码，先读 `01-` 和 `03-` 两份反解报告**，里面有大量"已证伪模型"清单。重新推导一遍会浪费两天然后得到同样的结论。
3. **`导出铁线` 是最终裁判**。它比任何单测都严格（123072 格逐格比）。改生成逻辑后必须跑它。
4. **遇到"数字对不上"先怀疑口径，不是怀疑实现**。这个项目历史上踩过 4 次同类坑：①`128` vs `824` vs `836` 三个设备口径 ②`5113` vs `5127` 两个行数口径 ③`85.2%` vs `84.31%` 两个覆盖率口径 ④`C3 100%` vs `68.29%` 两个节点范围口径。**每个数字都要问"分母是什么"。**
5. **写代码时别忘了 `domain/` 的零 IO 铁律**。想在里面 `import openpyxl` 的时候，说明这个逻辑该放 `infrastructure/`。

---

## 十、一句话总结交接状态

**能跑、能验收、能复现** —— 十一条能力的域层全部有 golden 测试锁定，前端十一页签全部可用，CLI 17 个子命令全部可跑。**下一件该做的事不是写代码，是先和用户确认 v1.0 要不要做 3D 与施工状态**（这两项都已被用户明确排到"后面再说"）。
