# 元研究：通用深度研究体系设计 — 系统性评估报告

> **评估日期**：2026-05-23  
> **评估对象**：`docs/specs/2026-05-23-meta-research-deep-research-system.html`  
> **评估方法**：将文档拆解为 6 个模块，分别进行关键术语网络搜索、学术文献对标、工程可行性分析

---

## 综合评分

| 模块 | 评分 (1-5) | 简评 |
|------|:---:|------|
| 认识论与验证体系 (V0-V5) | 3.5 | 方向正确，缺少正交维度和多维置信度 |
| 多智能体角色与编排 | 4.0 | 角色分工领先业界，缺冲突裁决和结构隔离 |
| 研究对象本体论 | 3.5 | 单项目模型完善，互操作与溯源不足 |
| 状态机与协议设计 | 3.5 | 宏观正确，缺并行/暂停/死锁机制 |
| 新颖性判定体系 | 3.5 | 多维分解有理论自觉，量化路径模糊 |
| 产品定位与可行性 | 4.0 | 占位 Layer 4 空白，MVP 路径清晰 |
| **综合** | **3.7** | **概念设计领先主流 Deep Research 产品一个层级，工程落地层仍需大量补强** |

---

## 一、总体判断

### 核心优势

该文档是一份**认识论工程化的研究操作系统设计**，其核心创新在于：

1. **把"什么是知识"从哲学讨论变成了可执行协议** — V0-V5 验证阶梯 + falsification_conditions 字段
2. **Research Program 作为一等公民** — 从"单次调研任务"提升到"长期可追踪知识生产"
3. **信号驱动的非线性状态机** — 比现有产品的黑盒循环更透明可审计
4. **研究对象化** — Question/Hypothesis/Evidence/Claim/Review 构成可追溯链路

这些设计让它与 Google Gemini Deep Research、OpenAI Deep Research、Perplexity 等产品产生本质区别：**现有产品是 "Research as a Service"（一次性报告交付），该系统是 "Research as an Operating System"（可验证的知识生产基础设施）**。

### 核心风险

1. **认识论立场混用**：同时采用 Popper 式证伪条件和 Bayesian 式 confidence，存在哲学张力
2. **单 FSM 无法支撑并行假设验证**：真实科研中多假设同时推进是常态
3. **复杂度过高**：六层架构 + 9 角色 + V0-V5 + 状态跳转表，用户迁移成本大
4. **自动化可靠性存疑**：V 阶梯评分、新颖性评分若由 LLM 自判，可能产生系统性偏差

---

## 二、分模块详细评估

### 模块 1：认识论与可验证性体系 (V0-V5)

#### 学术对标

| 对标概念 | 核心要义 | 对系统的启示 |
|---------|---------|------------|
| Popper 证伪主义 | 科学理论须具可反驳性 | `falsification_conditions` 方向正确 |
| 贝叶斯认识论 | 信念是分级 credence，理性更新遵循贝叶斯定理 | 单一 confidence 不足 |
| TRL (Technology Readiness Level) | 9 级技术成熟度，每级有退出准则 | V0-V5 是合理类比 |
| 可复现性危机 | Reproducibility ≠ Replicability | V3 未区分两者 |
| Oxford CEBM 证据等级 | 按研究设计类型分级，可升降级 | 缺少研究设计轴 |

#### 问题清单

| # | 问题 | 严重度 | 说明 |
|---|------|:---:|------|
| 1 | **V3 未区分"可复现"与"已被独立复现"** | 高 | 复现危机核心教训：提供材料 ≠ 独立团队复现成功 |
| 2 | **单一 0.0-1.0 置信度过于简化** | 高 | 无法区分认识论不确定性 vs 随机性不确定性；缺区间和更新日志 |
| 3 | **缺少元分析/系统综述层级** | 中 | CEBM 最高层是 RCT 系统综述，该体系无对应 |
| 4 | **缺少"研究设计类型"正交轴** | 中 | V4 可能基于单个 underpowered 观察性研究 |
| 5 | **Popper 反概率 vs Bayesian confidence 立场矛盾** | 中 | confidence 是"暂时证确"还是"后验概率"未明确 |
| 6 | **falsification_conditions 缺少 enforce 机制** | 中 | 无 ad hoc 救场追踪；可写成不可操作的空话 |
| 7 | **缺少证据间依赖结构** | 中 | 多条同源证据不应简单叠加 |
| 8 | **无时间衰减机制** | 低 | V1 来源可能已被 retract |

#### 改进建议

1. **拆分 V3 为两级**：
   - `V3-r`: Reproducible in principle（artifact 齐全）
   - `V3-i`: Independently replicated（≥1 独立复现）

2. **引入 `evidence_design_level`（CEBM/GRADE 映射）**，与 verification_level 构成二维矩阵

3. **confidence 改为向量 + 区间 + 更新日志**：
   ```yaml
   confidence:
     truth: [0.65, 0.80]        # 命题为真的后验区间
     replicability: 0.7         # 独立复现成功概率
     generalization: 0.5        # scope 外仍成立的概率
     update_log: [...]          # 每次证据变更的 delta
   ```

4. **强化 falsification_conditions schema**：
   ```yaml
   falsification_conditions:
     - condition: "若 X 在条件 Y 下出现"
       risk_level: high | medium | low
       tested: true | false
       test_outcome: survived | refuted | rescued_by_ad_hoc
   ```

---

### 模块 2：多智能体角色与编排

#### 学术对标

| 对标概念 | 说明 |
|---------|------|
| Kahneman 对抗性协作 | 对立双方需中立调解人，事前锁定预测 |
| Robin (Nature 2026) | AI 科研多 agent 系统 |
| InternAgent | 生成-验证双阶段 |
| CoThinker | 认知劳动分工理论 |
| A2A Protocol v1.0 | Google Agent-to-Agent 标准 |

#### 问题清单

| # | 问题 | 严重度 |
|---|------|:---:|
| 1 | **缺少 Adjudicator/Mediator 角色** — Critic 与 Theorist 同域，缺程序正义第三方 | 高 |
| 2 | **AI 担任 Research Lead 存在系统性风险** — LLM 过早收敛、确认偏误 | 高 |
| 3 | **Critic 缺乏结构性隔离** — 若共享 base model 和 evidence 视图，批判可能是 performative | 高 |
| 4 | **缺少 Analyst/Statistician** — 结果解释环节无独立角色 | 中 |
| 5 | **缺少 Reproducibility Curator** — 可复现性应有专人负责 | 中 |
| 6 | **Research Lead 与 Research Orchestrator 职能重叠** | 中 |
| 7 | **PI 审批点粒度不足** — 只有"关键实验"和"最终发布"两个 gate | 中 |
| 8 | **冲突裁决协议缺失** — Theorist vs Critic 僵持时无明确处理机制 | 高 |

#### 改进建议

1. **AI Research Lead → Staff Officer**；PI 保留 stage authority

2. **引入 Tiered Approval Ladder**：
   - L0 自动：证据收集、draft hypothesis
   - L1 异步 PI：scope 变更、中等成本实验
   - L2 同步 PI：claim V1→V2、跨项目知识写入
   - L3 委员会：伦理、人体、外部发布

3. **三级冲突裁决协议**：
   - Level 1：对象内协商（自动设计可区分实验）
   - Level 2：Adjudicator 程序裁决（preregistration）
   - Level 3：人类裁决（PI 选择并存/挂起/改问题）

4. **Red Team Critic 机制**：盲审、异构模型、轮换，防止 critic 被"驯化"

---

### 模块 3：研究对象本体论

#### 学术对标

| 标准/系统 | 核心范式 | 与本模型差距 |
|----------|---------|------------|
| FAIR 原则 | Findable, Accessible, Interoperable, Reusable | F 40%, A 50%, I 30%, R 45% |
| W3C PROV-DM | Entity + Activity + Agent 三元溯源 | Agent 贯穿不足，Activity 未充分建模 |
| CERIF | Person + Org + Project + Publication + Link Entity | 缺 Org/Publication/Funding |
| ORKG | Problem-Method-Result-Contribution + Templates | 缺 Contribution/Template 机制 |
| CER Framework | Claim-Evidence-Reasoning | Reasoning 未一阶化 |

#### 问题清单

| # | 问题 | 严重度 |
|---|------|:---:|
| 1 | **核心对象列表与关系图不一致** — VerificationPlan、KnowledgeNode 在关系图出现但未定义 | 高 |
| 2 | **CER 的 Reasoning 未一阶化** — 推理链只是字段不是独立对象 | 高 |
| 3 | **文献/外部知识缺少一阶对象** — 难表达"文献证据"和"已有 Claim 引用" | 中 |
| 4 | **Agent/Method/Dataset/Workflow 缺失** | 中 |
| 5 | **Provenance 不够细** — 缺 W3C PROV 的 wasDerivedFrom、wasAttributedTo | 中 |
| 6 | **跨项目复用机制缺失** — 无 global ID、entity resolution | 中 |
| 7 | **关系过度线性** — 难表达 derived claims、replication、multi-method triangulation | 中 |

#### 改进建议

1. **引入 `Reasoning`（或 `Warrant`）作为独立对象**，使 Evidence→Claim 完全符合 CER 三元组
2. **所有对象携带 PROV 三元组**：`wasGeneratedBy(Activity, timestamp)`, `wasAttributedTo(Agent)`
3. **补充高价值对象类型**：Publication, Dataset, Method, AnalysisWorkflow, Concept(SKOS)
4. **为核心对象分配 persistent URI/ID**，建立 Application Profile 对齐 Dublin Core + PROV-O

---

### 模块 4：状态机与协议设计

#### 学术对标

| 对标系统 | 适用场景 | 对比 |
|---------|---------|------|
| Kepler/Taverna/Galaxy | 科学工作流执行与溯源 | 工具执行层成熟，认知编排层弱 |
| Temporal.io | 持久化工作流引擎 | 长期暂停/恢复设计成熟 |
| OODA Loop | 观察-判断-决策-行动 | 与研究循环概念同构 |
| LangGraph | Agent 状态机编排 | 有 checkpoint/HITL，无科研语义 |

#### 问题清单

| # | 问题 | 严重度 |
|---|------|:---:|
| 1 | **单 FSM 无法表达多假设并行** — 假设 A 在 run_experiment，假设 B 在 collect_context，program "当前状态"无法表达 | P0 |
| 2 | **无死锁/无限循环终止协议** — 存在自然环路（collect→form→collect）但无 bound | P0 |
| 3 | **长期暂停/恢复未进入 Step 协议** — 跨月项目的 durable execution 空白 | P0 |
| 4 | **8 态与跳转表状态名不一致** — `analyze_results`、`revise_claim` 等游离节点 | P1 |
| 5 | **退出条件矛盾** — 跳转表要"至少一个假设"，协议示例要"至少2个互斥假设" | P1 |
| 6 | **LLM 可能自判"满足退出条件"** — 过程控制(P)与内容判断(C)未分离 | P1 |
| 7 | **缺少显式 `await_*` 状态** — 实验排队/人类审批应为 durable wait | P2 |

#### 改进建议

1. **采用三层编排模型**：
   ```
   Layer 1: Research Program Orchestrator (Entity Workflow, Temporal-style)
   Layer 2: Hypothesis Branch FSM (8 macro-states, 可并行)
   Layer 3: Step Activities (DAG / tool calls)
   ```

2. **引入 Loop Guard**：
   ```json
   {
     "loop_guard": {
       "max_visits_per_state": 5,
       "max_total_transitions": 40,
       "stagnation_window": 3,
       "escalation_target": "ask_human"
     }
   }
   ```

3. **退出条件分层化**：
   - `hard_gates[]` — schema 可校验，不满足则 blocking
   - `soft_scores[]` — LLM rubric，低于阈值则建议继续
   - `human_gates[]` — PI/专家签字

4. **借鉴 Temporal 的 durable wait**：`ask_approval` 产生 WorkflowPaused event + resume_token

---

### 模块 5：新颖性判定体系

#### 学术对标

| 对标概念 | 说明 |
|---------|------|
| Uzzi et al. (2013) | 高影响论文 = 高常规性 + 尾部非典型组合 |
| MDL/Kolmogorov Complexity | 压缩 = 发现规律，可作为结构新颖性度量基础 |
| Lakatos MSRP | 生产性 ≈ 研究纲领的"理论进步性" |
| CD/Disruption Index | 新知识是否取代旧知识成为引用基础 |
| OpenNovelty (2026) | 检索式 claim-level 新颖性判定 |
| Axiomatic Benchmark (2026) | 无单一指标满足全部新颖性公理 |

#### 问题清单

| # | 问题 | 严重度 |
|---|------|:---:|
| 1 | **未区分组织新颖性与全局新颖性** — 可能误判 rediscovery 为创新 | 高 |
| 2 | **缺少理论/概念新颖性维度** — "结构新颖性"不能替代范式位移 | 中 |
| 3 | **缺少颠覆/位移新颖性** — Disruption Index 是已验证的正交维度 | 中 |
| 4 | **"解释压缩度"量化路径不清** — MDL 有理论基础但未定义压缩对象 | 中 |
| 5 | **"生产性新颖性"本质是 ex-post 指标** — 与入库时即时判定存在时间张力 | 中 |
| 6 | **六维如何聚合未定义** — 等权平均会掩盖严重问题 | 中 |
| 7 | **双轴入库模型缺少 Rigor/Significance 轴** | 中 |

#### 改进建议

1. **拆分 scope 层**：`organization` vs `global`（全局新颖性强制走外部检索）

2. **聚合改用 claim-type 条件化**（非统一权重）：
   - 因果 claim：causal ≥ τ, empirical ≥ τ
   - 方法 claim：method ≥ τ, global ≥ τ
   - 发现 claim：empirical ≥ τ, global ≥ τ

3. **"解释压缩度"形式化为增量 MDL**：
   ```
   ΔMDL = [L(G_old) + L(ΔG|G_old)] - L(G_new)
   ```
   辅以 link-prediction surprise 和 Uzzi atypicality z-score

4. **双轴扩展为四维决策空间**：{novelty, verifiability, rigor, significance}

5. **生产性新颖性作为延迟验证维度**：初始用近端启发式（可分解性、预测跨度），后续贝叶斯更新

---

### 模块 6：产品定位与竞品对比

#### 市场定位分析

```
Layer 4: 机构知识 OS（跨项目 KG、验证协议、PI 治理）  ← 本系统目标 【基本空白】
Layer 3: 深度综合报告（Gemini DR、ChatGPT DR）        ← 已饱和
Layer 2: 专业 SR 工具（Elicit、Covidence）            ← 成熟
Layer 1: 文献发现（S2、Connected Papers）             ← 成熟
Layer 0: 学术 KG 基础设施（S2AG、ORKG）              ← 成熟
```

#### 本系统 vs 现有产品

| 维度 | 主流 Deep Research | 本系统 |
|------|-------------------|--------|
| 一等公民 | 报告、引用 | Question/Hypothesis/Evidence/Claim |
| 时间尺度 | 分钟~小时 | 周~月~年 |
| 知识沉淀 | 报告存档 | Claim Ledger + V 阶梯 + KG |
| 人机关系 | 人发起、读报告 | PI 审批门 + agent 分工 |
| 反例机制 | 偶尔提及 | Critic 角色 + counter_evidence + falsification 为必填 |

#### 关键风险

| 风险 | 缓解策略 |
|------|---------|
| 复杂度过高，用户不愿迁移 | Phase 1 只做对象工作台 |
| V 阶梯评分不可靠 | V0-V2 人工确认；V3+ 不自动化 |
| 与 DR 产品功能重叠感知 | slogan: "不是更好的报告，是更持久的知识" |
| 工程范围蔓延 | 严格 Phase 1→4 |

---

## 三、最高优先级改进清单 (Top 10)

| 优先级 | 改进项 | 影响范围 |
|:---:|------|------|
| 1 | 单 FSM → 三层编排（Program/Branch/Activity） | 架构 |
| 2 | 引入 Loop Guard + 死锁终止协议 | 架构 |
| 3 | AI Research Lead → Staff Officer；PI 保留决策权 | 治理 |
| 4 | V3 拆分为 V3-r（可复现）/ V3-i（已独立复现） | 验证 |
| 5 | 引入 Adjudicator + preregistration gate | 角色 |
| 6 | confidence 改为向量+区间+更新日志 | 数据模型 |
| 7 | 引入 Reasoning 一阶对象，完整对齐 CER | 本体 |
| 8 | 新颖性拆分 organization/global，强制外部检索 | 新颖性 |
| 9 | 建立 Tiered Approval Ladder (L0-L3) | 治理 |
| 10 | 引入 evidence_design_level (CEBM 映射) | 验证 |

---

## 四、MVP 落地建议

### 推荐范围（8-12 周）

| 必须有 | 可砍 | 可外包/集成 |
|--------|------|------------|
| 7 类核心对象 CRUD + 关系 | V3-V5 验证 | 文献检索 → S2 API / Consensus |
| 简化状态机（5 状态） | 9 角色全量 | 报告生成 → Gemini/ChatGPT DR |
| V0-V2 验证等级 | Experiment Registry | 筛选提取 → Elicit API |
| PI 审批门（1 个人类角色） | 跨项目 KG | 引用图 → Connected Papers |
| Object Inspector UI | Knowledge Map 可视化 | |
| Trace Log | 多 Program 权限域 | |

### MVP 成功标准

一个真实研究问题能在系统内完成完整对象链，且 6 个月后回看时**每个 claim 仍可追溯到 evidence 和 review** — 这是现有 Deep Research 产品做不到的。

### 差异化锚点

> **"未验证 claim 不能入库"** — 这一硬规则是本系统与"更好的 DR 报告生成器"的根本分界线。

---

## 五、总结

该文档展现了对科学认识论、知识管理和多智能体系统的**深度理解**，概念设计在当前 AI 产品格局中**领先一个层级**（从"调研工具"到"研究操作系统"）。最接近的竞品组合需要同时拼装 SciAgents + Elicit + ORKG + Temporal，但没有任何现有产品做到了**统一对象模型 + V 阶梯 + PI 审批门 + 跨项目状态机**的整合。

核心需要补强的是：**工程化层面的严谨性**（状态机形式化、并行/死锁、PROV 溯源、FAIR 对齐）和**产品层面的克制**（MVP 聚焦、用户迁移成本、与现有工具集成而非重建）。

---

## 附录：关键术语索引

| 术语 | 定义/来源 |
|------|---------|
| Falsificationism | Karl Popper 的科学哲学理论，主张科学理论必须具有可反驳性 |
| Bayesian Epistemology | 用概率论框架处理信念和证据的认识论学派 |
| TRL | NASA 提出的 9 级技术成熟度评估体系 |
| CEBM Level of Evidence | 牛津循证医学中心的证据等级体系 |
| Replication Crisis | 2010s 以来大量科学研究无法独立复现的系统性问题 |
| FAIR Principles | 科研数据管理的可发现、可访问、可互操作、可复用原则 |
| W3C PROV | 溯源数据模型标准，区分 Entity/Activity/Agent |
| CERIF | 欧洲科研信息通用格式 |
| ORKG | 开放研究知识图谱，结构化科研贡献 |
| CER Framework | Claim-Evidence-Reasoning 科学论证框架 |
| MDL (Minimum Description Length) | 基于数据压缩的统计学习原则 |
| Disruption Index | 度量新知识是否颠覆旧引用结构的指标 |
| Uzzi Atypicality | 度量知识组合非典型性的 z-score 方法 |
| Lakatos MSRP | 科学研究纲领方法论，区分进步/退化纲领 |
| OODA Loop | 军事决策循环：观察-判断-决策-行动 |
| Temporal.io | 持久化工作流引擎，支持长周期异步任务 |
| A2A Protocol | Google 发布的 Agent-to-Agent 互联协议 |
| Adversarial Collaboration | Kahneman 提出的对抗性科学协作方法 |
