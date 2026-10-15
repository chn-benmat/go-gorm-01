package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ข้อผิดพลาด:", err)
		os.Exit(1)
	}
}

func run() error {
	demoOnly := flag.Bool("demo", false, "แสดงตัวอย่างการค้นหาทั้ง 14 แบบ แล้วจบโปรแกรม")
	flag.Parse()

	config := loadDatabaseConfig()
	db, err := openDatabase(config)
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	fmt.Printf("เชื่อมต่อฐานข้อมูล %s ที่ %s:%s สำเร็จ\n", config.Name, config.Host, config.Port)
	fmt.Println("ใช้ข้อมูลเดิมในฐานข้อมูล")
	console := newConsole(db, os.Stdin, os.Stdout)
	if *demoOnly {
		return console.demo()
	}
	if err := console.run(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	fmt.Println("จบการทำงาน")
	return nil
}
