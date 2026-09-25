# gravity-reduce —— 重力勘探内业归算服务

把野外测得的绝对重力值 **gobs** 归算为**布格重力异常（Bouguer anomaly）**的纯后端 HTTP 服务。
无界面、无用户系统；只做重力测量数据归算，**不涉及**导航定位、卫星几何精度因子（GDOP/PDOP）、测绘派工或轨迹记录。

技术栈：**Go 1.22 + Gin**，依赖已 `vendor`，可在**只有一个本地 Go 基础镜像、完全离线**的条件下一次构建成容器。

---

## 1. 物理链条与符号约定

对每个测点，服务依次计算四个量：

| 步骤 | 量 | 公式 | 单位 |
|---|---|---|---|
| ① 理论正常重力 | γ(φ) | 国际正常重力公式（GRS80 / IAG1980，Somigliana） | m/s²（内部转 mGal） |
| ② 自由空气改正 | Δg_FA | `0.3086 · h` | mGal |
| ③ 布格板改正 | Δg_B | `0.04193 · ρ · h` | mGal |
| ④ 布格重力异常 | Δg_Bouguer | **`gobs − γ + Δg_FA − Δg_B`** | mGal（同时回传 m/s²） |

地形改正 Δg_T 在本服务**从简，缺省取 0**（合成式保留其位置：`… + Δg_T`）。

### 1.1 理论正常重力（纬度只在这一步进入计算）

Somigliana 闭合公式：

```
γ(φ) = γ_e · (1 + k·sin²φ) / √(1 − e²·sin²φ)
```

常数（GRS80）：

- γ_e = 9.7803267715 m/s²（赤道正常重力）
- k   = 0.00193185138639
- e²  = 0.00669437999013（参考椭球第一偏心率平方）
- φ 为地理（大地）纬度，单位：度

锚点（已在测试中锁定）：γ(0°)=9.7803267715、γ(45°)=9.8061991773、γ(±90°)=9.8321863685（m/s²）。

### 1.2 两项高程改正

- 自由空气改正（把观测值从测点高程归算回参考面，量级随高程**线性增大**）：

  ```
  Δg_FA = 0.3086 · h        # h 单位米，结果 mGal
  ```

  0.3086 mGal/m 对应垂直重力梯度 ≈ 3.086×10⁻⁶ s⁻²。

- 布格板改正（扣除测点与参考面之间那层无限平板物质的引力）：

  ```
  Δg_B = 0.04193 · ρ · h    # ρ 单位 g/cm³，h 单位米，结果 mGal
  ```

  系数 0.04193 来自无限平板 2πGρh：以 ρ 取 g/cm³、h 取米代入即得 0.04193·ρ·h mGal。

### 1.3 合成符号（钉死，不可翻号）

```
Δg_Bouguer = gobs − γ + Δg_FA − Δg_B
                       └ 正号      └ 负号
```

- **自由空气项为正、布格板项为负，二者符号相反、缺一不可**——绝不能只做其中一项。
- 一旦把自由空气符号取反，所有高地测点的异常会系统性下漂 `2·Δg_FA`；
  本仓库测试用“带符号项断言 + 独立公式交叉验证 + 翻号哨兵”三重手段抓住这种错误。

---

## 2. 单位链（务必保持一致）

```
1 Gal   = 1 cm/s²  = 1×10⁻² m/s²
1 mGal  = 1×10⁻³ Gal = 1×10⁻⁵ m/s²
1 m/s²  = 1×10⁵ mGal
```

- 输入 **gobs** 是绝对重力，单位 **m/s²（SI）**；γ 由公式算出也是 **m/s²**。
- 两项改正 Δg_FA、Δg_B 直接就是 **mGal**。
- 合成前，服务在唯一的单位出入口 `internal/units` 把 gobs、γ 统一换算成 mGal，
  **绝不让 m/s² 与 mGal 两种量纲的数悄悄相加**。
- 响应中 γ 与布格异常**同时回传 mGal 和 m/s²**；m/s² 侧保留 11 位小数以匹配 mGal 分辨率。

---

## 3. HTTP 接口

基址 `/api/v1`，仅 JSON。服务监听 `PORT`（默认 `8080`）。

### 3.1 单点归算 `POST /api/v1/reduce`

请求：

```json
{ "gobs": 9.8060, "h": 200, "phi": 45.0, "rho": 2.67 }
```

- `gobs` 绝对重力观测值，m/s²（必填，>0）
- `h` 测点高程，米（必填；0 合法，负值表示低于参考面）
- `phi` 地理纬度，度（必填，[-90, 90]）
- `rho` 中间层平均密度，g/cm³（必填，>0）

响应（节选，含四个核心量）：

```json
{
  "inputs": { "gobs_m_s2": 9.806, "h_m": 200, "phi_deg": 45, "rho_g_cm3": 2.67 },
  "normal_gravity":          { "mgal": 980619.920257, "m_s2": 9.80619920257 },
  "free_air_correction":     { "magnitude_mgal": 61.72,    "signed_mgal": 61.72 },
  "bouguer_slab_correction": { "magnitude_mgal": 22.39062, "signed_mgal": -22.39062 },
  "terrain_correction_mgal": 0,
  "bouguer_anomaly":         { "mgal": 19.409123, "m_s2": 0.00019409123 },
  "formula": "Delta_g_Bouguer = gobs - gamma + Delta_g_FA - Delta_g_B (+ Delta_g_T=0)",
  "signs": { "free_air": "正（+Δg_FA …）", "bouguer_slab": "负（-Δg_B …）", "terrain": "零（…0）" }
}
```

`magnitude_mgal` 是改正量大小（恒非负），`signed_mgal` 是真正代入合成式的带符号项（FA 正、板项负）。

### 3.2 高程扫描 `POST /api/v1/scan`

固定 gobs/φ/ρ，给一列高程，返回布格异常随高程变化的点列。
**每个点都由真实公式逐点计算**（实现上是对每个高程调用一次完整 `Reduce`），不是写死的直线；
改密度会改变点列斜率（布格板梯度不同），测试据此防止退化。

请求：

```json
{ "gobs": 9.8060, "phi": 45.0, "rho": 2.67, "heights": [0, 100, 200, 350] }
```

> 扫描接口用 `heights` 数组、不接受单点字段 `h`（同时给会被 400 拒绝）。

响应 `points[]` 每项含 `h_m`、`free_air_correction_mgal`、`bouguer_slab_correction_mgal`、`bouguer_anomaly{mgal,m_s2}`。

### 3.3 测线/测网批量归算与质量诊断 `POST /api/v1/lines/reduce`

一次收下**一条或多条测线**。每个测点除原来的 `gobs/h/phi/rho` 外，还要带测区
平面直角坐标 `easting_m`（东向，米）、`northing_m`（北向，米），并建议带沿线
里程 `station_m`（桩号，米）。服务对每个点**逐点复用 §1 的单点归算**拿到布格异常，
再在测线层做三类质控：

1. **沿测线水平梯度诊断**：先按里程理顺测点先后（不假定输入有序），逐段算
   `梯度 =（后点布格异常 − 前点布格异常）/ 水平距离`（mGal/km），超过
   `gradient_threshold_mgal_per_km` 的段标 `suspicious`，给出两端点、梯度与超出量。
   同位置重复布点（水平距离为 0）不报 Inf、不崩溃，而以 `kind:"coincident"`
   作为明确数据异常报出。
2. **测线两两交点差**：所有测线 i<j 两两配对，由两条线的平面折线段求交，
   判断交点是否落在两侧**实测段范围内**，再在每条线上按相邻测点布格异常线性插值，
   给出交点坐标、两侧插值（含所用测点段与插值参数 `parameter_t`）、交点差
   `difference_mgal = A−B` 及是否超 `intersection_tolerance_mgal`。平行、共线、
   延长线相交但越界、任一线降级等情形一律 `status:"no_intersection"` 并带原因码，
   **绝不硬凑交点差**。
3. **线内统计画像**：点数、均值、样本标准差（n−1）、最大/最小值及对应测点，
   全部由该线各点**真实归算结果**算出。

降级不是拒绝：单点测线、里程缺失/重复的线仍返回各点真实归算与统计，但
`status:"degraded"` 并带 `degrade_reasons`，梯度诊断 `status:"skipped"`，
涉及它的交点配对报 `line_degraded`。

平面坐标与里程**只用于测线几何与诊断，不参与物理归算**——整体平移一条线
（东向/北向加常量）各点布格异常与线内梯度诊断完全不变，已由测试锁定。

> 本接口只做重力测线的归算与质控：不做地图瓦片渲染、不做测线可视化绘图、
> 不涉及派工调度或人员轨迹。

请求：

```json
{
  "gradient_threshold_mgal_per_km": 30.0,
  "intersection_tolerance_mgal": 0.5,
  "lines": [
    { "id": "L1", "points": [
      { "id": "L1-0", "gobs": 9.8060, "h": 100, "phi": 45, "rho": 2.67,
        "easting_m": 0, "northing_m": 0, "station_m": 0 },
      { "id": "L1-1", "gobs": 9.8058, "h": 100, "phi": 45, "rho": 2.67,
        "easting_m": 100, "northing_m": 0, "station_m": 100 }
    ]},
    { "id": "L2", "points": [
      { "id": "L2-0", "gobs": 9.8060, "h": 100, "phi": 45, "rho": 2.67,
        "easting_m": 50, "northing_m": -50, "station_m": 0 },
      { "id": "L2-1", "gobs": 9.8059, "h": 100, "phi": 45, "rho": 2.67,
        "easting_m": 50, "northing_m": 50, "station_m": 100 }
    ]}
  ]
}
```

- `gradient_threshold_mgal_per_km` 必填，**正数**（mGal/km）。
- `intersection_tolerance_mgal` 必填，**非负**（mGal）。
- 线 `id`、点 `id` 必填且各自唯一；两个平面坐标必填；`station_m` 选填，
  整条线要么都给（定序），要么都不给（里程无法定序，该线诊断降级）。

响应 `lines[]` 含 `points`（已按里程排序的逐点归算）、`gradient.segments[]`、
`statistics`；`pairs[]` 每对含上述交点信息或无交点原因。
无交点原因码：`line_degraded` / `zero_length_line` / `parallel` /
`collinear` / `intersection_out_of_range`。

输入问题沿用既有错误风格（400 非法 JSON/未知字段；422 物理不合理或缺字段，
字段路径定位到具体线/点，如 `lines[0].points[1].easting_m`）。

### 3.4 内置手工验算示例 `GET /api/v1/sample`

返回中纬度、200 m 测点（见下节），便于人工核对。

### 3.5 健康检查 `GET /healthz` → `200 {"status":"ok",...}`

### 3.6 错误格式

非法/缺失输入**明确拒绝**，不返回看似正常的结果：

- `400`：不是合法 JSON、含未知字段、单端点用错字段（如 scan 传 `h`）。
- `422`：物理不合理或缺字段。形如：

```json
{ "error": { "message": "中间层平均密度必须为正数（g/cm³），不能为零或负", "field": "rho" } }
```

覆盖：纬度超出 [-90,90]、密度不为正（≤0）、缺失高程/任意必填项、采样序列缺失或为空、
序列含 NaN、任意字段为 NaN/±Inf、gobs 非正。注意 `h=0` 是**合法**输入，已与“缺失 h”区分。

---

## 4. 内置示例与手工验算

测点：φ=45°N，h=200 m，ρ=2.67 g/cm³，gobs=9.80600000 m/s²。

```
gobs   = 9.80600000 m/s² = 980600.000 mGal
γ(45°) = 9.80619920 m/s² = 980619.920 mGal
Δg_FA  = 0.3086 × 200        = 61.720 mGal
Δg_B   = 0.04193 × 2.67 × 200 = 22.39062 mGal
Δg_Bouguer = 980600.000 − 980619.920 + 61.720 − 22.39062 = 19.409123 mGal
          = 0.00019409123 m/s²
```

---

## 5. 被测试锁定的关键关系（逐条可执行）

位于 `internal/gravity/gravity_test.go` 与 `internal/api/api_test.go`：

1. **高程归零**：h=0 时 Δg_FA 与 Δg_B 同时为 0，布格异常恰等于 `gobs − γ`。
2. **高程翻倍两项同增**：其余不变，h→2h 时 Δg_FA、Δg_B 都翻倍，γ 不变。
3. **密度翻倍仅板项变**：ρ→2ρ 时只有 Δg_B 翻倍，Δg_FA 与 γ 毫不受影响。
4. **仅改纬度**：只有 γ 变化，两项高程改正保持不变。
5. **自由空气符号方向（重点）**：带符号项 FA 必须为正、板项必须为负；
   并用独立公式交叉验证 + “翻号结果哨兵”抓住符号取反（高地系统性漂移）。
6. **扫描真实性**：每点等于对该高程单独归算的结果；改变密度点列必须变化（写死直线过不了）。

---

## 6. 代码结构（按职责拆模块）

```
cmd/server/main.go              启动、端口、Gin 模式
internal/units/units.go         mGal ↔ m/s² 单位换算（唯一出入口）
internal/gravity/
  normal_gravity.go             国际正常重力公式（仅此处用纬度）
  corrections.go                自由空气改正、布格板改正、地形(=0)
  reduction.go                  合成布格异常（符号钉死）+ 高程扫描
internal/validate/validate.go   物理合理性与输入完备性校验
internal/survey/                测线/测网批量归算与质量诊断（独立职责层）
  survey.go                     领域类型：测线、点、梯度段、配对、统计、原因码
  validate.go                   批量请求结构/物理校验（逐点复用 validate.Point）
  line.go                       测点组织、按里程排序、逐点复用 gravity.Reduce、统计
  gradient.go                   沿测线水平梯度诊断（含零距离退化）
  intersect.go                  平面折线段求交 + 交点处线性插值
  process.go                    批量编排：逐线处理、测线两两配对
internal/api/                   HTTP：router / handlers / dto
  lines_dto.go / lines_handlers.go / lines_views.go   测线批量接口
internal/sample/sample.go       内置手工验算示例
vendor/                         固化依赖（离线构建）
```

批量层**复用而非重写**单点物理链条与单位换算：它只调用 `gravity.Reduce` /
`gravity.NormalGravity` / `gravity.FreeAirCorrection` / `gravity.BouguerSlabCorrection`
与 `units` 包，原单点接口、高程扫描、内置示例、错误返回风格保持不变。

---

## 7. 测线层被测试锁定的关键关系

位于 `internal/survey/survey_test.go` 与 `internal/api/lines_api_test.go`：

1. **平面平移不变**：整体沿东向/北向平移，各点布格异常、点序、梯度/可疑段/
   零距离段与水平距离全部不变。
2. **居中插点不告警**：在直线段中部插入里程居中、异常与线性趋势一致的点，
   不新增可疑段（各小段梯度与原段一致）。
3. **抬高触发两侧告警**：某点观测量不变、高程明显调离邻点，其左右两段都被标可疑，
   梯度数值用独立公式 `(0.3086−0.04193ρ)·Δh/距离` 交叉验证，且两侧符号相反。
4. **平行线无交点**：无论测点如何摆，返回 `no_intersection/parallel`，不产生交点差。
   共线（`collinear`）、延长线交点越界（`intersection_out_of_range`）同样拒绝凑数。
5. **等值交点差为零**：正交两线交点在测点段正中（`parameter_t=0.5`），两侧插值相等，
   交点差恰为 0；交点恰在已测点上（t=0/1）时同样成立。
6. **系统偏移交点差非零**：一条线整体抬高常量高程，交点差按
   `A−B = −(0.3086−0.04193ρ)·Δh` 呈可预期的非零系统偏移并标记超容差。
7. **退化输入**：同位置重复布点报 `coincident`（梯度为 0 而非 Inf/NaN）；
   单点测线归算/统计照常但梯度跳过、配对降级；里程缺失或重复定序降级；
   请求级缺坐标、缺阈值、空线、重复 ID、NaN/Inf 等带字段原因拒绝。

---

## 8. 运行、测试、构建

### 本地（需 Go 1.22）

```bash
go test ./...            # 全部自动化测试，可独立执行
go test -race ./...      # 带竞态检测
go run ./cmd/server      # 默认 :8080
```

或 `make test / make test-race / make run / make vet / make fmt`。

### 容器（只需本地有 Go 基础镜像，离线可建）

```bash
# 默认 FROM golang:1.22；本地标签不同就用 --build-arg 覆盖
docker build -t gravity-reduce .
docker build -t gravity-reduce --build-arg GO_IMAGE=golang:1.22.12-bookworm .

docker run --rm -p 8080:8080 gravity-reduce
curl -s http://127.0.0.1:8080/healthz
```

构建特点：单阶段（只需要这一个本地镜像，不再额外拉运行时镜像）、非 root 运行、
`CGO_ENABLED=0` 静态二进制、`-mod=vendor` + `GOPROXY=off` 纯离线、内置 `HEALTHCHECK`。

快速自检：

```bash
curl -s -XPOST localhost:8080/api/v1/reduce -H 'Content-Type: application/json' \
  -d '{"gobs":9.8060,"h":200,"phi":45,"rho":2.67}'
```
