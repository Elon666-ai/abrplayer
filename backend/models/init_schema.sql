-- Database schema for backend service.
--
-- The database name is configured at runtime via conf/backend.{env}.json
-- (Mysql.DbName) — this script does NOT create or select a database.
--
-- Usage:
--   1. Create the database manually using the name from your config, e.g.
--        CREATE DATABASE IF NOT EXISTS <your_db_name>
--            CHARACTER SET utf8mb4 COLLATE utf8mb4_general_ci;
--   2. Apply this schema:
--        mysql -h <host> -u <user> -p <your_db_name> < init_schema.sql

-- ===========================================
-- 1️⃣ 拉流开始统计表 (startplaystat)
-- ===========================================
CREATE TABLE IF NOT EXISTS statstartplay (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    gameTypeId VARCHAR(64) DEFAULT NULL COMMENT '游戏类型ID',
    gameUserId VARCHAR(64) NOT NULL COMMENT '游戏用户ID',
    gameRound VARCHAR(64) DEFAULT NULL COMMENT '局次/轮次编号',
    geoLocation VARCHAR(64) DEFAULT NULL COMMENT '地理位置',
    cdnName VARCHAR(64) DEFAULT 'tencent' COMMENT 'CDN名称',
    protocol VARCHAR(32) DEFAULT 'webrtc' COMMENT '拉流协议',
    quality VARCHAR(32) DEFAULT '1' COMMENT '拉流清晰度',
    streamName VARCHAR(128) DEFAULT NULL COMMENT '流名称',
    isSucceed TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否拉流成功',
    reason VARCHAR(255) DEFAULT NULL COMMENT '失败原因',
    userAgent VARCHAR(255) DEFAULT NULL COMMENT 'http请求的User-Agent',
    intervalFromLastPlay INT DEFAULT 0 COMMENT '距离上次play的间隔时间(ms)',
    waitMiliTime INT DEFAULT 0 COMMENT '首次拉流等待时间(ms)',
    retryCount INT DEFAULT 0 COMMENT '重试次数',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_createdAt (createdAt),
    INDEX idx_cdn_stream (cdnName, streamName),
    INDEX idx_user (gameUserId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='拉流开始统计表';

-- ===========================================
-- 2️⃣ 播放结束统计表 (endplaystat)
-- ===========================================
CREATE TABLE IF NOT EXISTS statendplay (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    gameTypeId VARCHAR(64) DEFAULT NULL COMMENT '游戏类型ID',
    gameUserId VARCHAR(64) NOT NULL COMMENT '游戏用户ID',
    gameRound VARCHAR(64) DEFAULT NULL COMMENT '局次/轮次编号',
    geoLocation VARCHAR(64) DEFAULT NULL COMMENT '地理位置',
    cdnName VARCHAR(64) DEFAULT 'tencent' COMMENT 'CDN名称',
    streamName VARCHAR(128) DEFAULT NULL COMMENT '流名称',
    playDuration INT DEFAULT 0 COMMENT '播放时长(ms)',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_createdAt (createdAt),
    INDEX idx_cdn_stream (cdnName, streamName),
    INDEX idx_user (gameUserId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='播放结束统计表';


-- ===========================================
-- 3️⃣ 播放卡顿统计表 (playlagstat)
-- ===========================================
CREATE TABLE IF NOT EXISTS statplaylag (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    gameTypeId VARCHAR(64) DEFAULT NULL COMMENT '游戏类型ID',
    gameUserId VARCHAR(64) NOT NULL COMMENT '游戏用户ID',
    gameRound VARCHAR(64) DEFAULT NULL COMMENT '局次/轮次编号',
    geoLocation VARCHAR(64) DEFAULT NULL COMMENT '地理位置',
    cdnName VARCHAR(64) DEFAULT 'tencent' COMMENT 'CDN名称',
    streamName VARCHAR(128) DEFAULT NULL COMMENT '流名称',
    lagDuration INT DEFAULT 0 COMMENT '卡顿持续时间(ms)',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    INDEX idx_createdAt (createdAt),
    INDEX idx_cdn_stream (cdnName, streamName),
    INDEX idx_user (gameUserId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='播放卡顿统计表';


-- ===========================================
-- 4️⃣ 录像成功率表 (recordstat)
-- ===========================================
CREATE TABLE IF NOT EXISTS statrecord (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    gameTypeId VARCHAR(64) DEFAULT NULL COMMENT '游戏类型ID',
    gameRound VARCHAR(64) DEFAULT NULL COMMENT '局次/轮次编号',
    isSucceed TINYINT(1) DEFAULT 0 COMMENT '是否录像成功(1成功,0失败)',
    reason VARCHAR(256) DEFAULT NULL COMMENT '失败原因',
    playbackUri VARCHAR(512) DEFAULT NULL COMMENT '录像回放地址',
    recordDuration INT DEFAULT 0 COMMENT '录像时长(s)',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '记录时间',
    INDEX idx_createdAt (createdAt),
    INDEX idx_gameType (gameTypeId),
    INDEX idx_round (gameRound)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='录像成功率统计表';


-- ===========================================
-- 5️⃣ 回放成功率表 (playbackstat)
-- ===========================================
CREATE TABLE IF NOT EXISTS statplayback (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    gameTypeId VARCHAR(64) DEFAULT NULL COMMENT '游戏类型ID',
    gameUserId VARCHAR(64) NOT NULL COMMENT '用户ID',
    geoLocation VARCHAR(128) DEFAULT NULL COMMENT '地理位置(国家/城市)',
    playbackUri VARCHAR(512) DEFAULT NULL COMMENT '回放视频地址',
    isSucceed TINYINT(1) DEFAULT 0 COMMENT '是否回放成功(1成功,0失败)',
    reason VARCHAR(256) DEFAULT NULL COMMENT '失败原因',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '记录时间',
    INDEX idx_createdAt (createdAt),
    INDEX idx_user (gameUserId)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='回放成功率统计表';

-- ===========================================
-- 5️⃣ 现场回合统计表 (statfieldround)
-- ===========================================
CREATE TABLE IF NOT EXISTS statfieldround (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    status ENUM('going', 'done', 'cancel') DEFAULT 'done' COMMENT '局状态',
    fieldName VARCHAR(128) DEFAULT 'GSP2W_FIELD' COMMENT '现场名称',
    roundId VARCHAR(128) NOT NULL UNIQUE COMMENT '游戏局号',
    roundStartTime DATETIME DEFAULT NULL COMMENT '局开始时间',
    roundEndTime DATETIME DEFAULT NULL COMMENT '局结束时间',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updatedAt DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_field_round (fieldName, roundId),
    INDEX idx_round (roundId),
    INDEX idx_createdAt (createdAt)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='现场局次统计表';

-- ===========================================
-- Hourly Monitoring Table (hourly_videostat)
-- ===========================================
CREATE TABLE IF NOT EXISTS videostat_hourly (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    statHour DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '统计小时 (整点)',
    fieldName VARCHAR(128) DEFAULT NULL COMMENT '现场名称',
    streamPath VARCHAR(128) DEFAULT NULL COMMENT '现场流名称',

    requestStreamTotal BIGINT DEFAULT 0 COMMENT '拉流请求总数',
    streamSuccessNum BIGINT DEFAULT 0 COMMENT '拉流成功次数',
    avgLoadTime FLOAT DEFAULT 0 COMMENT '平均拉流加载时延(ms)',
    reconnectCount BIGINT DEFAULT 0 COMMENT '重连次数(<15s内)',
    avgStreamSuccessRate FLOAT DEFAULT 0 COMMENT '平均拉流成功率(%)',
    lagDurationTotal BIGINT DEFAULT 0 COMMENT '卡顿总时长(ms)',
    playDurationTotal BIGINT DEFAULT 0 COMMENT '播放总时长(ms)',
    avgLagRate FLOAT DEFAULT 0 COMMENT '平均卡顿率(%)',
    lagCount BIGINT DEFAULT 0 COMMENT '严重卡顿次数(>2000ms)',
    avgPlayDuration FLOAT DEFAULT 0 COMMENT '平均播放时长(s)',
    recordTotal BIGINT DEFAULT 0 COMMENT '录像次数',
    recordSuccessNum BIGINT DEFAULT 0 COMMENT '录像成功数',
    avgRecordSuccessRate FLOAT DEFAULT 0 COMMENT '录像成功率(%)',
    playbackTotal BIGINT DEFAULT 0 COMMENT '回放次数',
    playbackSuccessNum BIGINT DEFAULT 0 COMMENT '回放成功数',
    avgPlaybackSuccessRate FLOAT DEFAULT 0 COMMENT '回放成功率(%)',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_hour (statHour)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每小时监控统计表';


-- ===========================================
-- 6️⃣ 每日视频播放聚合表 (daily_videostat)
-- ===========================================
CREATE TABLE IF NOT EXISTS videostat_daily (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    statDate DATE NOT NULL UNIQUE COMMENT '统计日期 (YYYY-MM-DD)',
    fieldName VARCHAR(128) DEFAULT NULL COMMENT '现场名称',
    streamPath VARCHAR(128) DEFAULT NULL COMMENT '现场流名称',

    requestStreamTotal INT DEFAULT 0 COMMENT '拉流总次数',
    streamSuccessNum INT DEFAULT 0 COMMENT '拉流成功次数',
    streamSuccessRate DECIMAL(6,2) DEFAULT 0 COMMENT '拉流成功率(%)',
    avgLoadTime FLOAT DEFAULT 0 COMMENT '平均拉流加载时延(ms)',
    reconnectCount BIGINT DEFAULT 0 COMMENT '重连次数(<15s内)',

    playDurationTotal BIGINT DEFAULT 0 COMMENT '播放总时长（毫秒）',
    lagDurationTotal BIGINT DEFAULT 0 COMMENT '卡顿总时长（毫秒）',
    lagCount INT DEFAULT 0 COMMENT '卡顿次数',
    lagRate DECIMAL(6,2) DEFAULT 0 COMMENT '卡顿率(%)',
    avgPlayDuration INT DEFAULT 0 COMMENT '平均播放时长（秒）',

    requestRecordTotal INT DEFAULT 0 COMMENT '录像总次数',
    recordSuccessNum INT DEFAULT 0 COMMENT '录像成功次数',
    recordSuccessRate DECIMAL(6,2) DEFAULT 0 COMMENT '录像成功率(%)',

    playbackTotal INT DEFAULT 0 COMMENT '回放总次数',
    playbackSuccessNum INT DEFAULT 0 COMMENT '回放成功次数',
    playbackSuccessRate DECIMAL(6,2) DEFAULT 0 COMMENT '回放成功率(%)',

    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP,
    updatedAt DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_statDate (statDate)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每日视频播放统计聚合表';

-- ===========================================
-- 6️⃣ 每局视频播放聚合表 (videostat_round)
-- ===========================================
CREATE TABLE IF NOT EXISTS videostat_round (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    roundId VARCHAR(128) NOT NULL UNIQUE COMMENT '局号',
    fieldName VARCHAR(128) DEFAULT NULL COMMENT '现场名称',
    roundStartTime DATETIME DEFAULT NULL COMMENT '直播开始时间',
    roundEndTime DATETIME DEFAULT NULL COMMENT '直播结束时间',
    liveDuration BIGINT DEFAULT 0 COMMENT '直播时长(秒)',
    requestStreamTotal BIGINT DEFAULT 0 COMMENT '拉流总次数',
    streamSuccessNum BIGINT DEFAULT 0 COMMENT '拉流成功次数',
    reconnectCount BIGINT DEFAULT 0 COMMENT '重连次数',
    avgLoadTime DOUBLE DEFAULT 0 COMMENT '平均开播时延(ms)',
    recordDurationTotal BIGINT DEFAULT 0 COMMENT '录像时长总和(秒)',
    videoCompletionRate DOUBLE DEFAULT 0 COMMENT '录像完整率(%)',
    streamSuccessRate DOUBLE DEFAULT 0 COMMENT '拉流成功率(%)',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP,
    updatedAt DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_round (roundId),
    INDEX idx_createdAt (createdAt)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每局(按RoundId)直播聚合统计表';

CREATE TABLE IF NOT EXISTS info_field_streams (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    fieldName VARCHAR(128) NOT NULL UNIQUE COMMENT '现场名称',
    streamPath VARCHAR(256) NOT NULL COMMENT '对应的流路径',
    description VARCHAR(256) DEFAULT NULL COMMENT '备注信息',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updatedAt DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_field (fieldName)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='现场与直播流路径映射表';

CREATE TABLE IF NOT EXISTS info_fields (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    fieldName VARCHAR(128) NOT NULL UNIQUE COMMENT '现场名称',
    fieldType VARCHAR(128) NOT NULL COMMENT '现场类型(快彩/慢彩)',
    description VARCHAR(256) DEFAULT NULL COMMENT '备注信息',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updatedAt DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    INDEX idx_field (fieldName)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='现场信息表';

CREATE TABLE IF NOT EXISTS info_system_settings (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    configKey VARCHAR(128) NOT NULL UNIQUE COMMENT '配置Key',
    configValue VARCHAR(512) NOT NULL COMMENT '配置值',
    description VARCHAR(256) DEFAULT NULL COMMENT '备注信息',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updatedAt DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY uniq_config_key (configKey)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='系统配置表';

-- ===========================================
-- 流状态监控表 (statStreamStatus)
-- ===========================================
CREATE TABLE IF NOT EXISTS statStreamStatus (
    id BIGINT AUTO_INCREMENT PRIMARY KEY COMMENT '自增主键',
    streamPath VARCHAR(128) NOT NULL COMMENT '流名称',
    startTime DATETIME DEFAULT NULL COMMENT '流启动时间',
    breakStartTime DATETIME DEFAULT NULL COMMENT '断流开始时间',
    breakRecoverTime DATETIME DEFAULT NULL COMMENT '断流恢复时间',
    status ENUM('active', 'broken', 'recovered', 'ended') DEFAULT 'active' COMMENT '流状态',
    duration BIGINT DEFAULT 0 COMMENT '推流持续时长(毫秒)',
    breakDuration BIGINT DEFAULT 0 COMMENT '中断持续时长(毫秒)',
    totalBreakCount INT DEFAULT 0 COMMENT '累计中断次数',
    lastHeartbeat DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '最近心跳时间',
    createdAt DATETIME DEFAULT CURRENT_TIMESTAMP COMMENT '创建时间',
    updatedAt DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '更新时间',
    UNIQUE KEY uniq_stream (streamPath),
    INDEX idx_status (status),
    INDEX idx_lastHeartbeat (lastHeartbeat)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='流状态监控表';
