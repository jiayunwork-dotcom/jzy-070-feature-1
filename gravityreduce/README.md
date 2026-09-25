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

### 3.3 测线/测网批量归算与质控 `POST /api/v1/lines/reduce`

一次收下一条或多条测线。每条线是一串测点；测点除单点观测量（`gobs/h/phi/rho`）外，
还要带平面直角坐标 `easting_m`/`northing_m`（东向/北向，米，必填），
并可标注沿线里程/桩号 `station_m`（米；一条线要么全给、要么全不给）。
服务对每个测点**复用既有单点归算链条**拿到布格异常，然后做三类线级质控：

1. **沿测线水平梯度检查**：测点先按里程理顺（送入顺序任意），逐段计算
   `g = (Δg_B[To] − Δg_B[From]) / 水平距离`（mGal/m），超过
   `gradient_threshold_mgal_per_m` 的段标记为可疑并给出超阈量；
   同位置重复布点（距离为 0）不作为无穷大，而单列到 `data_anomalies`。
2. **测线对交点差**：所有测线两两组合，把测线视为按里程连接测点的折线求平面交点，
   判定交点落在两条线的测量范围内后，在每条线上对相邻测点布格异常做**线性插值**，
   给出交点坐标、两侧插值、`difference_mgal = A − B` 及是否超出
   `intersection_tolerance_mgal`。平行、共线（重叠/相离）、交点在测量段之外、
   某条线无法定序等情形一律 `status=no_valid_crossing` 并带 `reason`/`reason_detail`，
   **不硬凑交点差**（共线端点相接是唯一交点，正常给出）。
3. **线内统计画像**：点数、均值、总体标准差、最大/最小值及对应测点——
   全部由该线各点真实归算结果算出。

请求：

```json
{
  "gradient_threshold_mgal_per_m": 0.05,
  "intersection_tolerance_mgal": 0.5,
  "lines": [
    {"id": "L1", "points": [
      {"id": "a", "easting_m": 0,   "northing_m": 0,   "station_m": 0,   "gobs": 9.8060, "h": 100, "phi": 45, "rho": 2.67},
      {"id": "b", "easting_m": 200, "northing_m": 0,   "station_m": 200, "gobs": 9.8060, "h": 100, "phi": 45, "rho": 2.67},
      {"id": "c", "easting_m": 400, "northing_m": 0,   "station_m": 400, "gobs": 9.8060, "h": 100, "phi": 45, "rho": 2.67}
    ]},
    {"id": "L2", "points": [
      {"id": "d", "easting_m": 300, "northing_m": -100, "station_m": 0,   "gobs": 9.8060, "h": 100, "phi": 45, "rho": 2.67},
      {"id": "e", "easting_m": 300, "northing_m": 100,  "station_m": 200, "gobs": 9.8060, "h": 100, "phi": 45, "rho": 2.67}
    ]}
  ]
}
```

响应要点：`lines[]` 含 `status`（`ok`/`degraded`）、`notes`（降级原因）、
按里程排好序的 `points[]`（每个点都是完整单点归算结果）、`gradient`、`statistics`；
`pairs[]` 为全部两两线对结果，`status=intersects` 时 `crossings[]` 列出每个交点。

降级而不拒绝（仍给逐点归算与统计，`status=degraded` 并在 `notes` 说明）：
单点测线、缺里程、里程重复、含零距离重复布点。硬拒绝（422，字段定位到具体测点）：
缺坐标/观测量、纬度越界、密度非正、里程只标一部分、线号/点号重复、阈值非法等。

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

测线批量接口的错误风格与单点一致：JSON/未知字段 400，物理/缺字段 422，
`field` 精确定位（如 `lines[1].points[3].easting_m`）；
单点测线、缺里程、重复里程、零距离重复布点等撑不起某类诊断的数据情形不拒绝，
在线级 `status=degraded` + `notes` 中带原因降级，逐点归算与统计画像始终照给。

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
7. **平面平移不变**：整条测线沿东向/北向平移常量，各点布格异常、逐段梯度诊断、
   统计画像均不变；两条线一起平移，交点差也不变（归算只取决于纬度、高程、密度，与平面位置无关）。
8. **居中插点不告警**：在直线段中部插入里程居中、观测量符合线性趋势的新测点，不新增可疑段；
   把某点高程明显调离邻点，则在它左右**两侧**各触发一个梯度告警。
9. **平行线无交点**：平行（对齐/错开/一长一短任意摆法）、共线重叠、共线相离、
   支撑线交点落在测量段之外，一律报“无有效交点”且 `crossings` 为空，不硬凑；
   共线端点相接给出唯一交点。
10. **等值交点差为零 / 系统偏移交点差非零**：两线交点附近异常相等时交点差为 0；
    把一条线整体抬高一个高程常量，交点差等于 `(0.3086−0.04193·ρ)·Δh` 并超容限。
11. **退化输入不糊弄**：零距离重复布点报数据异常而非无穷大；单点测线、缺里程、
    重复里程带原因降级；缺坐标、部分里程、非法观测量带字段定位拒绝。

---

## 6. 代码结构（按职责拆模块）

```
cmd/server/main.go            启动、端口、Gin 模式
internal/units/units.go       mGal ↔ m/s² 单位换算（唯一出入口）
internal/gravity/
  normal_gravity.go           国际正常重力公式（仅此处用纬度）
  corrections.go              自由空气改正、布格板改正、地形(=0)
  reduction.go                合成布格异常（符号钉死）+ 高程扫描
internal/validate/validate.go 物理合理性与输入完备性校验
internal/survey/              测线/测网批量归算与质量诊断（复用单点链条，不重写物理）
  types.go        领域类型（Point/Line/Thresholds，梯度/交点/统计结果结构）
  validate.go     批级与线级输入校验（复用 validate.Point）
  reduction.go    对单点 gravity.Reduce 的唯一复用点
  ordering.go     测线组织与按里程定序（乱序理顺、缺/重里程降级）
  gradient.go     沿测线水平梯度检查 + 零距离退化数据异常
  statistics.go   线内布格异常统计画像
  geometry.go     折线求交、共线/平行判定、段内线性插值
  intersections.go 测线两两交点差编排与无交点原因分类
  process.go      批量处理总编排
internal/api/                 HTTP：router / handlers / dto（含 lines_* 批量接口）
internal/sample/sample.go     内置手工验算示例
vendor/                       固化依赖（离线构建）
```

---

## 7. 运行、测试、构建

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

测线批量自检（两条交叉测线，查梯度与交点差）：

```bash
curl -s -XPOST localhost:8080/api/v1/lines/reduce -H 'Content-Type: application/json' -d '{
  "gradient_threshold_mgal_per_m": 0.05,
  "intersection_tolerance_mgal": 0.5,
  "lines": [
    {"id":"L1","points":[
      {"id":"a","easting_m":0,"northing_m":0,"station_m":0,"gobs":9.8060,"h":100,"phi":45,"rho":2.67},
      {"id":"b","easting_m":400,"northing_m":0,"station_m":400,"gobs":9.8060,"h":100,"phi":45,"rho":2.67}]},
    {"id":"L2","points":[
      {"id":"d","easting_m":200,"northing_m":-100,"station_m":0,"gobs":9.8060,"h":100,"phi":45,"rho":2.67},
      {"id":"e","easting_m":200,"northing_m":100,"station_m":200,"gobs":9.8060,"h":100,"phi":45,"rho":2.67}]}
  ]
}'
```
