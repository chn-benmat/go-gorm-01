package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"regexp"
	"time"
	_ "time/tzdata"

	sqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type databaseConfig struct {
	Host, Port, User, Password, Name string
}

func envOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func loadDatabaseConfig() databaseConfig {
	return databaseConfig{
		Host:     envOrDefault("DB_HOST", "127.0.0.1"),
		Port:     envOrDefault("DB_PORT", "3306"),
		User:     envOrDefault("DB_USER", "root"),
		Password: envOrDefault("DB_PASSWORD", ""),
		Name:     envOrDefault("DB_NAME", "store_db"),
	}
}

func openDatabase(config databaseConfig) (*gorm.DB, error) {
	// ชื่อฐานข้อมูลใช้ placeholder ไม่ได้ จึงจำกัดตัวอักษรก่อนนำไปใช้ใน SQL
	if !regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`).MatchString(config.Name) {
		return nil, fmt.Errorf("ชื่อฐานข้อมูลต้องมี 1-64 ตัวอักษร และใช้ได้เฉพาะ a-z, A-Z, 0-9, _")
	}
	location, err := time.LoadLocation("Asia/Bangkok")
	if err != nil {
		return nil, err
	}
	driverConfig := sqldriver.NewConfig()
	driverConfig.User = config.User
	driverConfig.Passwd = config.Password
	driverConfig.Net = "tcp"
	driverConfig.Addr = net.JoinHostPort(config.Host, config.Port)
	driverConfig.ParseTime = true
	driverConfig.Loc = location
	driverConfig.Timeout = 5 * time.Second
	driverConfig.ReadTimeout = 10 * time.Second
	driverConfig.WriteTimeout = 10 * time.Second
	driverConfig.Params = map[string]string{"charset": "utf8mb4"}

	// ต่อ server ก่อน เพื่อสร้าง store_db กรณียังไม่มีฐานข้อมูล
	server, err := sql.Open("mysql", driverConfig.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("ตั้งค่าการเชื่อมต่อ MySQL: %w", err)
	}
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("เชื่อมต่อ MySQL ไม่สำเร็จ ตรวจ XAMPP และ DB_HOST/DB_PORT/DB_USER/DB_PASSWORD: %w", err)
	}
	if _, err := server.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+config.Name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
		return nil, fmt.Errorf("สร้างฐานข้อมูล: %w", err)
	}

	driverConfig.DBName = config.Name
	db, err := gorm.Open(mysql.Open(driverConfig.FormatDSN()), &gorm.Config{
		TranslateError: true,
		Logger:         logger.Default.LogMode(logger.Silent),
		NowFunc:        func() time.Time { return time.Now().In(location) },
	})
	if err != nil {
		return nil, fmt.Errorf("เปิดฐานข้อมูลด้วย GORM: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	if err := db.Set("gorm:table_options", "ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci").AutoMigrate(
		&Category{}, &Product{}, &Customer{}, &Order{},
	); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("AutoMigrate: %w", err)
	}
	return db, nil
}
