# 🧩 backend

### 📘 Introduction

`backend` 服务主要功能如下：

* 接收 **player** 与 **recorder** 的上报数据；
* 自动补充 `geoLocation`、`createdAt`；
* 校验上报字段合法性；
* 高效写入 **MySQL**；
* 提供基础健康监控与日志记录；
* 提供 **视频播放与录像指标可视化监控**。

---

### 🎯 核心指标定义

| 指标                              | 计算公式                        | 说明           |
| ------------------------------- | --------------------------- | ------------ |
| **拉流成功率** `streamSuccessRate`   | 拉流成功次数 ÷ 拉流尝试总次数 × 100%     | 表示客户端拉流成功的比例 |
| **拉流卡顿率** `lagRate`             | 卡顿时长(ms) ÷ 播放总时长(ms) × 100% | 播放过程中卡顿占比    |
| **平均播放时长** `avgPlayDuration`    | 播放总时长 ÷ 播放成功次数（秒）           | 播放失败不计入      |
| **录像成功率** `recordSuccessRate`   | 成功录像文件数 ÷ 期望录像文件数 × 100%    | 上传失败或太短视为失败  |
| **回放成功率** `playbackSuccessRate` | 成功回放次数 ÷ 回放请求总数 × 100%      | 文件损坏等导致失败计入  |

统计时间：

* 每天 **00:00:01** 自动统计前一天数据；
* 支持 **按小时 / 按天 / 按局次** 的指标统计。

---

### 📊 Monitor Requirements

| 指标                              | 统计周期          | 说明             |
| ------------------------------- | ------------- | -------------- |
| **Live Play Success Rate**      | 每小时 / 每天 / 每局 | 成功拉流次数 / 拉流总次数 |
| **Live Lag Rate**               | 每小时 / 每天 / 每局 | 卡顿时长 / 播放总时长   |
| **Video Playback Success Rate** | 每天            | 成功回放次数 / 回放总次数 |
| **Recording Success Rate**      | 每天            | 成功录像次数 / 总录像次数 |

---

### 🧩 API Reference

---

### 💾 MySQL Initialization

数据库名由 `conf/backend.{env}.json` 中的 `Mysql.DbName` 字段决定（示例配置 `abrplayer`）。
先手动创建该数据库，再应用表结构脚本：

```bash
# 1. Create database (replace <db_name> with Mysql.DbName from your config)
mysql -h 127.0.0.1 -u root -p -e "CREATE DATABASE IF NOT EXISTS <db_name> CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;"

# 2. Apply schema
mysql -h 127.0.0.1 -u root -p <db_name> < backend/models/init_schema.sql
```

---

### 🚀 Deployment

**部署模式：单实例部署**
每个环境一个实例（共三个）：

| 环境     | 用途            | 配置      |
| ------ | ------------- | ------- |
| `local` | Local | 2C / 4G |
| `test` | Test | 2C / 4G |
| `uat` | UAT | 2C / 4G |
| `stag` | Staging | 2C / 4G |
| `prod` | Production | 2C / 4G |

---

#### 🏗️ 构建镜像

```bash
sudo docker build -t backend:v1.0.0 -f Dockerfile .
```

---

#### 🔍 调试运行

```bash
# 临时进入容器调试
sudo docker run -it --rm --net=host backend:v1.0.0 /bin/sh
```

或后台运行：

```bash
sudo docker run -d --name abrplayer-backend --net=host backend:v1.0.0
```

---

### ⚙️ CI/CD 发布流程

使用 **Git Tag** 管理版本发布：

```bash
# 标记发布版本
git tag release_pro_v1.0.0-alpha.1

# 推送到远程
git push origin release_pro_v1.0.0-alpha.1
```

CI 服务器根据 tag 自动构建 Docker 镜像并推送至仓库。

---

### 🧠 目录结构建议

```bash
backend/
├── main.go
├── go.mod
├── conf/
│   └── backend.{APP_ENV}.json
├── models/
│   └── init_schema.sql
├── apis/
├── services/
├── tasks/
├── tracer/
├── utils/
└── README.md
```

Supported `APP_ENV` values are case-insensitive: `local`, `dev`, `test`,
`stag`, and `prod`. Legacy `uat` runtime input remains accepted for existing deployments. Runtime config is loaded from `conf/backend.{APP_ENV}.json`.
The public base URL table for these environments is owned by the AbrPlayer
backend DB and can be edited from the `admin-web` AbrPlayer view.

| APP_ENV | Label | Public base URL |
| ------- | ----- | --------------- |
| `local` | local-env | `http://localhost:8088` |
| `dev` | UAT-env | `https://videostat-uat.example.com` |
| `test` | test-env | `https://videostat-test.example.com` |
| `stag` | STAG-env | `https://videostat-stag.example.com` |
| `prod` | PROD-env | `https://videostat-prod.example.com` |
