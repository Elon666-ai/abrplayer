# ABRPlayer.js (v1.0.3) 技术说明文档

## 1. 产品概述
**ABRPlayer.js** 是一个基于腾讯云 `TCPlayer` SDK 封装的高级 WebRTC 播放器控制器。它专为弱网环境下的直播场景设计，通过内置的 **自适应码率 (ABR)** 状态机、**故障自愈**机制以及**视觉无缝优化**，解决了原生 WebRTC 在网络波动时容易卡死、黑屏或画质不佳的问题。
*   **Demo**: [https://abrplayer.example.com/](https://abrplayer.example.com/)
*   **适用场景**: 高频互动的 WebRTC 直播、弱网环境（4G/3G 切换）、多码率自适应播放。

---

## 2. 核心功能特性

### 2.1 智能 ABR (自适应码率)
播放器不依赖服务端的 ABR 协议（如 HLS 的 m3u8），而是在**客户端**实现了一套完整的 ABR 决策逻辑：

*   **初始选流 (Cold Start)**：
    *   **记忆优先**：读取 Cookie 中上次播放的清晰度。
    *   **测速兜底**：若无记忆，尝试通过静态图片测速（需配置）。
    *   **安全兜底**：若测速失败，默认选择中间画质或指定的兜底流（如 `720p`），避免起播失败。
*   **混合带宽估算**：
    *   **被动统计 (Passive)**：在播放视频时，直接利用 WebRTC `getStats` 返回的实时接收码率 (`receiveKbps`) 作为带宽参考，**零额外流量消耗**。
    *   **主动探测 (Active)**：仅在当前处于最低画质（Audio 模式）且渴望升级时，才会启动低频图片下载测速，以探测网络上限。
    *   **平滑算法**：采用 **滑动窗口 (Moving Average)** 和 **EWMA (指数加权移动平均)** 过滤网络抖动。
*   **升降级策略 (Fast Drop, Slow Rise)**：
    *   **快速降级 (Direct Jump)**：当监测到带宽严重不足（如 4G 转 3G），不再逐级尝试，而是根据当前实测带宽，**一步到位**跳到能承载的最佳流（例如从 1080p 直接降到音频）。
    *   **稳定升级 (Step Up)**：当网络长期稳定（>30秒）且带宽充裕时，尝试向上提升一级画质。

### 2.2 视觉体验优化 (Pseudo-Seamless Switching)
针对 WebRTC 切换必须断开重连的特性，实现了**“最后一帧冻结”**方案：
1.  **冻结**：切换触发瞬间，将当前 `<video>` 的最后一帧绘制到上层 Canvas。
2.  **重连**：后台销毁旧实例，建立新连接。
3.  **解冻**：监听新流的 `playing` 事件，一旦画面渲染，立即移除 Canvas。
*   **效果**：用户感知为画面短暂定格（类似网络卡顿）后恢复播放，消除了原本的黑屏闪烁体验。

### 2.3 异常检测与故障自愈
*   **死流/卡顿检测 (Stall Detection)**：
    *   监听播放器 `1009/1010` 事件及解码帧率。若卡顿超过 **5000ms**，判定为死流。
    *   **动作**：强制触发降级重试。
*   **错误黑名单 (Error Blacklisting)**：
    *   捕获严重错误（如 Code 14 解码失败、连接失败）。
    *   **自动拉黑**：将报错的流标记为 `Online: False`，ABR 逻辑在后续运行中会自动跳过该流。
    *   **兜底保护**：默认兜底流（如 `audio_64kbps`）享有豁免权，即使报错也不会被拉黑，确保有最后一条生命线进行无限重试。

### 2.4 设备能力适配
*   **HEVC/H.265 智能检测**：启动时通过 `MediaCapabilities` API 检测浏览器是否支持 HEVC 硬解。
*   **动态梯队**：如果设备不支持 HEVC，播放器会自动将流阶梯中的 HEVC 流剔除，只在 H.264 流之间切换，避免解码错误。

---

## 3. 流阶梯设计 (Stream Ladder)

| 档位 | 名称 | 码率 (Bitrate) | 编码 | 说明 |
| :--- | :--- | :--- | :--- | :--- |
| **0** | `audio_64kbps` | 128 kbps | AAC | **兜底流**，纯音频，无画面 |
| **1** | `360p_h264` | 350 kbps | H.264 | 最低视频档 |
| **2** | `720p_hevc` | 400 kbps | HEVC | 高效编码，低带宽高清 |
| **3** | `720p_h264` | 950 kbps | H.264 | 通用高清 |
| **4** | `1080p_h264` | 2000 kbps | H.264 | 最高画质 |

---

## 4. 关键配置参数 (Config)

| 参数名 | 默认值 | 作用 |
| :--- | :--- | :--- |
| `ABR_SWITCH_COOLDOWN_MS` | 30000 | **冷却时间**。切换后 30s 内锁定，防止震荡。 |
| `MONITOR_INTERVAL_MS` | 1000 | **心跳频率**。每 1s 执行一次状态检查。 |
| `STALL_THRESHOLD_MS` | 5000 | **卡顿阈值**。缓冲 >5s 触发强制降级。 |
| `BITRATE_DOWN_FACTOR` | 0.6 | **降级因子**。接收码率 < 目标 * 0.6 触发降级。 |
| `BANDWIDTH_PROBE_INTERVAL_MS` | 8000 | **主动测速间隔**（仅在 Audio 模式下生效）。 |

---

## 5. 前端集成指南

### 5.1 文件引入顺序
```html
<!-- 1. 腾讯云 SDK -->
<script src="https://video.sdk.qcloudecdn.com/web/TXLivePlayer-1.3.4.min.js"></script>
<!-- 2. 工具库 -->
<script src="./js/v3/utils.js"></script>
<!-- 3. ABR 核心 -->
<script src="./js/v3/abrplayer.js"></script>
<!-- 4. 业务逻辑 -->
<script src="./js/v3/main.js"></script>
```

### 5.2 调用示例

```javascript
// 实例化
const player = new ABRPlayer('player-container-id');

// 绑定数据回调 (用于更新 UI)
player.onStats = (data) => {
    console.log(`Loss: ${data.video.calculatedLoss}, Bitrate: ${data.video.bitrate}`);
};

// 启动自动模式
// 参数1: null (自动初始选流)
// 参数2: true (开启 ABR)
player.start(null, true);

// 停止
// player.stop();
```

---

## 6. 技术局限性与未来展望

### 当前局限性：Hard Switch (硬切换)
目前的方案属于客户端侧的“硬切换”。
*   **机制**：`Disconnect` -> `Connect New URL`。
*   **现象**：切换时必须重新建立 TCP/UDP 连接和 DTLS 握手，耗时通常在 0.5s ~ 2.0s。虽然使用了 Canvas 遮盖黑屏，但画面会有明显的“定格-跳变”。
*   **原因**：受限于服务端架构（简单的多路转码流）和播放器 SDK 限制。

### 未来优化方向：Soft Switch (软切换)
要实现 Netflix/YouTube 级别的 0ms 无缝切换，需要升级到底层 **WebRTC Simulcast** 技术。

#### 方案：标准 Simulcast (WebRTC 原生方式)
1.  **推流端**：OBS 使用多路输出插件，同时推送 1080p, 720p, 360p 三路流。
2.  **服务端**：
    *   部署支持 Simulcast 的 SFU（如 **Mediasoup**, **SRS**, **LiveKit**）。
    *   服务端将三路流聚合为一个逻辑 Session。
3.  **播放端 (重构)**：
    *   放弃 `TCPlayer`，使用原生 `RTCPeerConnection` API。
    *   **切换逻辑**：不再销毁连接。通过信令发送 `setPreferredLayers({ spatialLayer: 2 })`。
    *   **效果**：服务端在现有连接通道内直接切换 RTP 包源，客户端解码器无缝衔接，实现 **0ms 黑屏、0ms 冻结** 的完美切换。

