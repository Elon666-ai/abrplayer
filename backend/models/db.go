package models

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	dbHdl   *gorm.DB
	dbMutex sync.Mutex
)

// InitDbConn 初始化数据库连接
func InitDbConn() error {
	CloseDbConn()

	// DSN: root:pass@tcp(127.0.0.1:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		settings.Mysql.User,
		settings.Mysql.Password,
		settings.Mysql.Host,
		settings.Mysql.Port,
		settings.Mysql.DbName,
	)

	// 自定义 GORM 日志器
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             500 * time.Millisecond, // 超过500ms标记为慢SQL
			LogLevel:                  logger.Warn,            // 只记录慢SQL与警告
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)

	// 初始化连接
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                                   newLogger,
		SkipDefaultTransaction:                   true, // 🚀 提高插入性能（禁用隐式事务）
		PrepareStmt:                              true, // 🚀 缓存SQL语句模板
		DisableForeignKeyConstraintWhenMigrating: true, // 迁移时不自动创建外键
	})
	if err != nil {
		log.Println("[DB] failed to connect mysqlMain:", err)

		backupHost := settings.Mysql.BackupHost
		if backupHost == "" {
			backupHost = settings.Mysql.Host
		}
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
			settings.Mysql.User,
			settings.Mysql.Password,
			backupHost,
			settings.Mysql.Port,
			settings.Mysql.DbName,
		)
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			Logger:                                   newLogger,
			SkipDefaultTransaction:                   true, // 🚀 提高插入性能（禁用隐式事务）
			PrepareStmt:                              true, // 🚀 缓存SQL语句模板
			DisableForeignKeyConstraintWhenMigrating: true, // 迁移时不自动创建外键
		})
		if err != nil {
			log.Println("[DB] failed to connect mysqlBackup:", err)
			return fmt.Errorf("[DB] failed to connect mysql: %w", err)
		}
	}

	dbHdl = db

	sqlDB, err := dbHdl.DB()
	if err != nil {
		return fmt.Errorf("[DB] failed to get sql.DB: %w", err)
	}

	// ===== 连接池配置 =====
	sqlDB.SetMaxIdleConns(50)
	sqlDB.SetMaxOpenConns(200)
	sqlDB.SetConnMaxLifetime(4 * time.Hour)
	sqlDB.SetConnMaxIdleTime(1 * time.Hour)

	// ===== 检测连接可用性 =====
	if err = sqlDB.Ping(); err != nil {
		return fmt.Errorf("[DB] initial ping failed: %w", err)
	}

	fmt.Println("[DB] connected successfully:", settings.Mysql.Host)
	// 自动迁移表结构
	// if err = dbHdl.AutoMigrate(&StartPlayStat{}, &EndPlayStat{}, &PlayLagStat{}, &RecordStat{}, &PlaybackStat{}, &FieldRoundStat{},
	// 	&HourlyVideoStat{}, &DailyVideoStat{}, &RoundVideoStat{}, &FieldStreamsInfo{}, &FieldsInfo{}, &StatStreamStatus{}); err != nil {
	// 	fmt.Println("error! auto migrate table failed: ", err)
	// 	return err
	// }

	if err = dbHdl.AutoMigrate(&SystemSetting{}); err != nil {
		fmt.Println("[WARN] auto migrate info_system_settings:", err)
	}

	return nil
}

// CloseDbConn 关闭连接
func CloseDbConn() {
	if dbHdl != nil {
		sqlDB, err := dbHdl.DB()
		if err == nil {
			sqlDB.Close()
		}
		dbHdl = nil
	}
}

// GetDbConn 获取连接（自动重连，加锁保证只初始化一次）
func GetDbConn() *gorm.DB {
	if dbHdl != nil {
		return dbHdl
	}
	dbMutex.Lock()
	defer dbMutex.Unlock()
	if dbHdl == nil {
		_ = InitDbConn()
	}
	return dbHdl
}

// ActiveDbConn 自动检测数据库连接是否断开
func ActiveDbConn() *gorm.DB {
	dbMutex.Lock()
	defer dbMutex.Unlock()
	if dbHdl == nil {
		_ = InitDbConn()
		return dbHdl
	}
	sqlDB, err := dbHdl.DB()
	if err != nil || sqlDB.Ping() != nil {
		log.Println("[DB] reconnecting after ping fail:", err)
		_ = InitDbConn()
	}
	return dbHdl
}

