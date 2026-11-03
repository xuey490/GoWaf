# GoWAF 多引擎/多面板适配评估报告

> 评估日期：2026-11-04
> 目标：从「宝塔 + Apache 专用」演进为「自动托管 80/443，转发给 Nginx 或 Apache」，支持 1Panel、小皮面板、原生 systemd 等环境，成为完整的 WAF 形态。

---

## 一、结论摘要（TL;DR）

**可行，且改动面比想象的小。** 核心结论四条：

1. **Go 二进制（数据面/控制台）已经是引擎无关的**——反向代理按 Host 分流、透传 X-Forwarded-*、TLS 终止、限流封禁，对后端是 Apache 还是 Nginx 完全无感。**所有耦合点都在 deploy/install.sh 一个文件里**（约 1900 行），这是唯一的改造主战场。
2. 真正的工作不是"重写"，而是**把 install.sh 重构为「面板探测 × 引擎适配器」两层结构**：Apache 适配器 = 现有逻辑原样搬入；新增 Nginx 适配器（原生 / 宝塔 / 1Panel 三种布局）。预计总量 5~8 人天，可分三期交付。
3. **1Panel 的 OpenResty 跑在 Docker 容器里**，适配策略与改配置文件不同——优先走「容器端口重映射」路线（宿主 80→WAF，容器 80 重映射到宿主 8080），比改容器内 nginx 配置更稳。这是四个环境里唯一需要新拓扑思路的。
4. **Windows（小皮 Windows 版 + IIS）也在范围内**：配置文件语法可复用现有适配器（小皮 Windows 就是标准 Apache/Nginx conf 布局），真正的增量是三块——① gowaf 需要 Windows 服务模式（唯一一处动 Go 代码的需求）；② IIS 用 appcmd 改 applicationHost.config 而非改配置文件，且**没有 mod_remoteip 等价物**（后端真实 IP 依赖 ARR，需如实降级）；③ 打包/防火墙/服务管理全套 Windows 化。增量约 2.5~3.5 人天。

---

## 二、现状盘点：耦合在哪里、干净在哪里

### 2.1 已经引擎无关的部分（不用动）

| 模块 | 现状 |
|---|---|
| `internal/proxy` | 纯 Go `httputil.ReverseProxy`，透传 Host、追加 XFF、带 `X-Forwarded-Proto: https` |
| `internal/edge` | 检测/限流/封禁/挑战，全部基于 HTTP 语义，与后端引擎无关 |
| `internal/proxy/router` | Host/SNI 分流，每站点独立 upstream（**天然支持"站点级上游"→ 未来双引擎并存零改动**） |
| 自环防护 | 上游端口命中自身监听端口即拒绝加载，通用 |
| 控制台 | 站点管理/防护配置/计划任务，无引擎概念 |

### 2.2 install.sh 里的耦合点（全部要动）

| 耦合点 | 位置 | 性质 |
|---|---|---|
| 硬编码宝塔路径 `/www/server/{panel,apache}` | L47-50 | 中 |
| `detect_env` 只认宝塔 Apache / 系统 Apache，遇 Nginx 直接 die（"Nginx 请勿直接运行"） | L217-260 | **高（主入口）** |
| `retarget_vhosts`：`<VirtualHost *:80>` → `127.0.0.1:8080`（Apache 语法专属） | L549 | 高 |
| `inject_remoteip`：mod_remoteip 加载 + `RemoteIPInternalProxy 127.0.0.1` + `SetEnvIf X-Forwarded-Proto HTTPS=on` | L490-520 | 高 |
| 宝塔强制 HTTPS 段修复（`RewriteCond %{SERVER_PORT} !^443$` 死循环，刚在 156 服务器实战过） | retarget_vhosts 内 | 中 |
| 证书发现：宝塔目录布局（`vhost/cert/<站点>/fullchain.pem`），按 SAN 匹配域名 | L131-210 | 中 |
| 服务控制：systemd + 宝塔 SysV 双轨 `apache_ctl` | L386 | 低（框架已好，加 nginx 版即可） |
| 配置校验/重载：`apachectl configtest` / `graceful` | 多处 | 低 |

> 结论：**数据面零改动，改造 100% 集中在部署脚本**。风险被天然隔离——适配失败不影响已部署的 156 服务器。

---

## 三、目标架构：两层适配器

```
install.sh（编排层，保持现有 install/rollback/uninstall/resync/doctor 命令骨架）
   │   （Windows 侧对应 install.ps1，复用同一套适配器函数约定）
   ├─ detect_stack()                 ← 新：面板探测 × 引擎探测
   │     面板：宝塔 / 1Panel / 小皮 / 原生 systemd
   │     引擎：apache / nginx / openresty
   │
   └─ Adapter 接口（每个引擎实现一份，bash 函数组约定）
        ├─ list_sites          站点域名清单（证书发现的输入）
        ├─ retarget            把站点监听从 80/443 改到 127.0.0.1:$UPSTREAM_PORT（幂等）
        ├─ inject_realip       真实 IP 透传（apache: mod_remoteip / nginx: realip 模块）
        ├─ fix_redirect_loop   引擎等价的"强制 HTTPS"死循环修复
        ├─ configtest          apache -t / nginx -t
        ├─ reload              graceful / nginx -s reload
        └─ cert_layout         各面板证书目录布局 → 统一交给现有 pick_cert 逻辑
```

**面板探测只决定"路径和重载方式"，引擎适配器决定"改什么配置"**——两维正交。宝塔 Apache / 宝塔 Nginx / 1Panel OpenResty / 原生 Nginx 就是四个组合，没有爆炸。

---

## 四、逐引擎适配要点与难点

### 4.1 Apache（现役，重构即可）
现有逻辑原样搬入 `adapter_apache()` 函数组，行为零变化。唯一新增：系统版 Apache（Debian `sites-enabled`）路径已在 detect_env 里有，保留。

### 4.2 原生 Nginx（systemd，最标准的场景）

| 事项 | 方案 |
|---|---|
| 站点发现 | `nginx -T` dump 解析 server 块（比猜 include 目录可靠），提取 `server_name` + `listen` |
| 监听改写 | `listen 80;` → `listen 127.0.0.1:8080;`；`listen [::]:80` 同理改环回或注释掉（WAF 暂不接管 IPv6 时必须处理，否则 nginx 起不来或双栈行为不一致） |
| **default_server 冲突** | **最大坑**：多个 server 块都有 `default_server` 标记，全部改到 8080 后 nginx 直接拒绝启动。改写时保留第一个、其余摘除标记 |
| 真实 IP | `set_real_ip_from 127.0.0.1; real_ip_header X-Forwarded-For; real_ip_recursive on;`（realip 模块发行版一般内置，需检测 `nginx -V` 的 `--with-http_realip_module`） |
| 强制 HTTPS 死循环 | Nginx 版等价物：站点常见 `if ($server_port !~ ^443)` 或 `$scheme = http` 跳转——与 Apache 301 事故同源，补 `map $http_x_forwarded_proto` 或改条件。**这是实战验证过的坑，nginx 站点大概率也有** |
| HTTP/2 | `listen 443 ssl http2` 改写后 http2 标记失去意义，WAF 到后端是 HTTP/1.1，需摘除（否则有的版本告警） |
| QUIC/HTTP3 | 若站点有 `listen 443 quic`（UDP），WAF 不接 UDP——**明确降级：提示用户关闭或保留 UDP 由源站直答**（写进 doctor 提示，不做自动改） |
| 校验/重载 | `nginx -t` + `nginx -s reload`（平滑，不断连接） |

### 4.3 宝塔 Nginx
- 站点配置在 `/www/server/panel/vhost/nginx/*.conf`，结构与宝塔 Apache 对应，cert 目录同宝塔（`vhost/cert`）→ 证书发现逻辑直接复用
- 软链 nginx 在 `/www/server/nginx/sbin/nginx`
- 强制 HTTPS 段与宝塔 Apache 同款模板（`$server_port` 判断），同款修复
- 宝塔「重新保存站点覆盖配置」的已知风险与 Apache 相同，resync 兜底（现成）

### 4.4 1Panel OpenResty（Docker）——唯一需要新拓扑

1Panel 的 OpenResty 是容器（`-p 80:80 -p 443:443`），站点配置在宿主 `/opt/1panel/apps/openresty/openresty/conf/conf.d/*.conf` 挂载进容器。两条路线对比：

| 路线 | 做法 | 评价 |
|---|---|---|
| **A. 端口重映射（推荐）** | WAF 占宿主 80/443；把 OpenResty 的发布端口从 `80:80` 改为 `8080:80`（1Panel 应用商店支持改映射，或改 compose）。**容器内配置一字不改**，WAF 上游填 `127.0.0.1:8080` | 改动最小、升级 OpenResty 不回退；宿主上 docker-proxy 监听 8080，WAF 直达 |
| B. 改 conf.d 的 listen | 与原生 Nginx 同款改写 | 也可行（配置挂载自宿主），但容器端口仍发布在 80 会冲突，还是要动映射——等于 A+额外步骤，无收益 |

**路线 A 的两个配套点**：
1. **realip 信任网段**：容器收到的连接源地址是 docker 网桥网关（172.16.0.0/12 段）而非 127.0.0.1，`set_real_ip_from` 必须写容器网段——由脚本探测 `docker network inspect` 自动生成
2. **健康检查/重载**：`docker exec <openresty> nginx -t && nginx -s reload`，服务名从 compose 容器名取

### 4.5 小皮面板 Linux（phpStudy Linux）
- 站点配置路径与宝塔同构：Nginx 在 `/www/server/panel/vhost/nginx/<域名>.conf`、Apache 在 `/www/server/panel/vhost/apache/<域名>.conf`——**宝塔适配器改改路径常量即复用**，证书目录沿用面板布局
- 面板「保存/重定向」操作会按模板重新生成整个 vhost 文件（官方行为，与宝塔同款覆盖风险），resync 兜底
- 探测函数识别到小皮布局时走宝塔适配器 + 路径覆盖，列为 P2 收尾

### 4.6 双引擎并存（加分项，数据面已支持）
WAF 路由表本来就是 per-site upstream。机器上同时有 Apache(8080) 和 Nginx(8081) 退守时，控制台把各站点上游指到对应端口即可——**这是比 cPanel 级方案更灵活的形态，且数据面零开发**。

### 4.7 Windows：小皮 Windows 版 + IIS

Windows 不是"又一个面板"，而是**一套平行的宿主环境**（服务模型、路径、防火墙、打包全不同）。分三块评估：

**A. 共同底座（所有 Windows 环境的前置）**

| 事项 | 方案 | 性质 |
|---|---|---|
| Windows 服务模式 | **gowaf 增加 `svc install/run/stop/uninstall` 子命令**（`golang.org/x/sys/windows/svc` 实现真 SCM 服务）。不用 NSSM/WinSW 第三方依赖，保住"单二进制"卖点 | **唯一动 Go 代码的需求**，约 1 天 |
| 主目录/数据 | `C:\ProgramData\gowaf`（data/logs/configs），部署脚本生成 config.yaml | 脚本 |
| 防火墙 | `netsh advfirewall` 放行 80/443/8899（80/443 通常已被 IIS/Apache 规则占位，检测后补） | 脚本 |
| 打包 | package.ps1 增加 Windows 分支：产出 zip（gowaf.exe + install.ps1 + builtin.yaml），install.ps1 = Windows 版编排脚本 | 脚本 |
| console open/close 临时放行 | Linux 用 iptables，Windows 用 `netsh advfirewall` 临时规则；计划任务中心清理事件的定时逻辑不变 | 脚本 |

**B. 小皮 Windows 版（phpstudy_pro）**

- 实测路径布局（已核实）：`<盘>:\phpstudy_pro\Extensions\Apache2.4.39\conf\vhosts\*.conf`、`Extensions\Nginx1.15.11\conf\vhosts\*.conf`（目录带版本号，探测时用 glob 匹配 `Extensions\Apache*\conf\vhosts`）
- **语法与通用 Apache/Nginx 适配器完全一致**——retarget/realip/强制 HTTPS 修复逻辑直接复用，只换路径发现和重载命令
- 服务控制：小皮面板自管进程（可能注册了 Apache 服务），脚本按「Windows 服务优先（`sc query`），无服务则走面板进程管理器重启」处理
- 证书：小皮 Windows 无统一 cert 目录（各站点自配），探测时提示用户在控制台手动上传或给目录——首版可要求 `--cert-dir` 显式指定
- 面板覆盖风险与 Linux 小皮相同，resync 兜底
- 工作量：约 1 天（复用为主）

**C. IIS（差异最大，需要独立适配器）**

| 事项 | 方案 | 难点 |
|---|---|---|
| 站点发现 | `appcmd list sites` + `appcmd list vdirs`，读 `%windir%\System32\inetsrv\config\applicationHost.config` 的 `<site><bindings>` | 低 |
| 监听改写 | appcmd 把 `*:80`/`*:443` 绑定改为 `127.0.0.1:8080`（保留 host header）；**https 绑定转 http**（WAF 终止 TLS，IIS 收明文即可，省掉 8080 上的证书搬运） | 中（绑定语法细节多，需逐站点处理 SNI） |
| http.sys | 改完绑定 http.sys 即释放 80/443，WAF 可绑定；IIS 检测配置变更自动生效，通常无需重启 | 低 |
| 重载/校验 | `appcmd` 改绑定即时生效；整站回滚 = 还原 applicationHost.config 备份 | 低 |
| **真实 IP** | **IIS 无 mod_remoteip 等价物**。后端日志/PHP 应用看到的是 127.0.0.1。可选：ARR（Application Request Routing）+ URL Rewrite 把 XFF 写回 REMOTE_ADDR——但 ARR 是重量级模块且与"改绑定"路线有耦合。**首版决策：不装 ARR**，WAF 侧不受影响（WAF 本来就是边缘，直接看到真实 IP），doctor 明示"后端应用将看到 127.0.0.1"的降级说明 | **最大短板，如实降级** |
| 强制 HTTPS 死循环 | IIS 站点的 https 跳转一般在 ASP/PHP 应用层或 URL Rewrite 规则里，改绑定后 `%HTTPS_OFF` 类条件可能死循环——doctor 增检测项，应用层规则留给用户 | 中 |
| 工作量 | 独立适配器 + install.ps1 Windows 分支 | 约 1.5~2 天 |

> Windows 合计增量：**2.5~3.5 人天**（svc 子命令 1 + 小皮 Windows 1 + IIS 1.5~2，打包脚本并入各阶段）。

---

## 五、关键技术风险与对策

| # | 风险 | 等级 | 对策 |
|---|---|---|---|
| 1 | default_server 冲突致 nginx 起不来 | 高 | 改写时去重（保一摘余）+ `nginx -t` 失败自动回滚文件（复用现有 backup/rollback 机制） |
| 2 | 强制 HTTPS 死循环（nginx 版 301 事故） | 高 | fix_redirect_loop 纳入 retarget 流程，幂等补条件 |
| 3 | 面板保存站点覆盖配置（复发） | 中 | 已有 resync + install.sh 固化修复模式，扩展到 nginx；doctor 增「漂移检测」 |
| 4 | 1Panel 容器网段 realip 错配 → 全站日志变网关 IP、限流打到网关 | 中 | docker network 探测自动生成 set_real_ip_from |
| 5 | 改坏配置站点全挂 | 中 | 现有 backup → 改写 → configtest → 失败即回滚 的链路通用化；reload 优先 graceful（nginx reload 本身平滑） |
| 6 | QUIC(UDP 443) 无法托管 | 低 | doctor 明示降级；不做自动处理 |
| 7 | 面板自身入口被 WAF 拦（1Panel 可用 nginx 反代面板端口） | 低 | 探测面板入口域名/端口，自动加入「跳过接管」清单 |
| 8 | 未知面板/手工魔改配置 | 低 | 探测失败 → 明确报错退出（保持现在"宁可不装不乱改"的底线） |
| 9 | **Windows**：gowaf 服务化失败/权限不足；IIS 绑定改错打不开站点 | 中 | `svc` 子命令完整实现 SCM 协议；applicationHost.config 改前全量备份 + appcmd 逐站验证 + 失败自动还原 |
| 10 | **Windows**：后端应用看到 127.0.0.1（无 realip） | 低（如实降级） | 首版不装 ARR，doctor 明示；限流/封禁不受影响（WAF 在边缘直接看到真实 IP） |

---

## 六、实施分期与工作量

| 阶段 | 内容 | 产出 | 工作量 |
|---|---|---|---|
| **P0 重构**（不动行为） | install.sh 拆出 `adapter_apache()` 函数组 + detect_stack()；全部现有命令回归测试（156 服务器实机验证） | 行为零变化的适配器框架 | 1~1.5 天 |
| **P1 原生 Nginx** | 站点发现(nginx -T)、listen 改写、default_server 去重、realip、强制 HTTPS 修复、nginx 版 apache_ctl | 洁净 systemd Nginx 环境一键安装 | 2~2.5 天 |
| **P2 宝塔 Nginx + 1Panel** | 宝塔 nginx 布局（复用证书发现）；1Panel 路线 A（compose 端口重映射 + 容器 reload + 网桥 realip） | 宝塔 nginx / 1Panel 环境一键安装 | 2~2.5 天 |
| **P3 收尾** | doctor 扩展（引擎/面板/漂移/QUIC 报告）、rollback 按引擎还原、README 更新、测试矩阵跑完 | 完整发布（Linux 全覆盖） | 1~1.5 天 |
| **P-W Windows** | ① `gowaf svc` Windows 服务子命令（Go，唯一动代码）；② install.ps1 Windows 编排 + package.ps1 zip 产出 + 防火墙；③ 小皮 Windows 适配（复用 Apache/Nginx 适配器换路径）；④ IIS 适配器（appcmd 绑定改写 + applicationHost.config 备份回滚） | Windows 全覆盖 | 2.5~3.5 天 |

**测试矩阵**（每格 = install → curl 验证 → rollback → uninstall 全链路）：
宝塔 Apache（现役 156 服务器当回归基准）/ 原生 Nginx / 宝塔 Nginx / 1Panel OpenResty / 小皮 Linux / 小皮 Windows（Apache + Nginx 各一轮）/ IIS。

---

## 七、明确不做 / 降级的范围

- ~~不做 Windows~~ → **已纳入（P-W 阶段）**，见 4.7 节
- **不做 cPanel/Plesk**：海外场景国内占比低，适配器接口留好，需求出现再接
- **IIS 不装 ARR**：后端真实 IP 降级为 127.0.0.1（WAF 侧不受影响），doctor 明示；有 ARR 需求的站点人工启用
- **QUIC/HTTP3 托管**：不做，doctor 提示降级
- **Docker 多容器编排场景**（1Panel 装了多个独立 Nginx 应用）：只适配「OpenResty 网站引擎」这个标准形态，其余 doctor 报告提示人工处理
- **IPv6 接管**：暂不接管 IPv6（WAF 只听 80/443 的 IPv4），脚本处理源站的 IPv6 listen 避免 nginx 起不来，IPv6 流量直连源站由 doctor 提示

---

## 八、总结

- **形态目标成立**：Go 数据面已就绪，Linux 侧改造收敛在部署脚本一层，架构风险低
- **最大的两个技术坑**（default_server 冲突、强制 HTTPS 死循环 nginx 版）都有明确对策，且后者已在 Apache 上实战修复过一次
- **1Panel 是拓扑变化而非代码难度**，端口重映射路线可以做到容器配置零侵入
- **Windows 纳入后总量 8~11.5 人天**：唯一的 Go 代码需求是 `gowaf svc` 服务子命令；小皮 Windows 复用现有适配器（仅换路径）；IIS 独立适配器 + 后端真实 IP 如实降级
- 建议节奏：P0 先做（对现役环境零风险），P1/P2 每期以一台真实环境验证后再合并，P-W 放最后（需要准备 Windows 实机）——与本项目一贯的「先实机验证再固化」节奏一致
