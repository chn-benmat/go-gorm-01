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
	seedOnly := flag.Bool("seed", false, "เพิ่มข้อมูลตัวอย่างหากฐานข้อมูลว่าง แล้วจบโปรแกรม")
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
	seeded, err := seedDatabaseDB(db)
	if err != nil {
		return fmt.Errorf("เพิ่มข้อมูลตัวอย่าง: %w", err)
	}
	if seeded {
		fmt.Println("เพิ่มข้อมูลตัวอย่าง: 4 ประเภท, 15 สินค้า, 8 ลูกค้า, 12 คำสั่งซื้อ")
	} else {
		fmt.Println("ใช้ข้อมูลเดิมในฐานข้อมูล")
	}
	console := newConsole(db, os.Stdin, os.Stdout)
	if *demoOnly {
		return console.demo()
	}
	if *seedOnly {
		return nil
	}
	if err := console.run(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	fmt.Println("จบการทำงาน")
	return nil
}
