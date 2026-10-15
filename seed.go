package main

import (
	"time"

	"gorm.io/gorm"
)

// เพิ่มข้อมูลตัวอย่างเฉพาะเมื่อทั้งสี่ตารางว่าง รวมรายการที่ Soft Delete แล้ว
func seedDatabaseDB(db *gorm.DB) (bool, error) {
	seeded := false
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, model := range []any{&Category{}, &Product{}, &Customer{}, &Order{}} {
			var count int64
			if err := tx.Unscoped().Model(model).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return nil
			}
		}
		categories := []Category{
			{Name: "แล็ปท็อป", Description: "คอมพิวเตอร์พกพาสำหรับเรียนและทำงาน"},
			{Name: "สมาร์ตโฟน", Description: "โทรศัพท์มือถือและแท็บเล็ต"},
			{Name: "อุปกรณ์เสริม", Description: "เมาส์ คีย์บอร์ด และอุปกรณ์เชื่อมต่อ"},
			{Name: "จอภาพและอุปกรณ์จัดเก็บข้อมูล", Description: "จอภาพ SSD และอุปกรณ์สำรองข้อมูล"},
		}
		for i := range categories {
			if err := createCategoryDB(tx, &categories[i]); err != nil {
				return err
			}
		}
		products := []Product{
			{Name: "MacBook Air", Price: 34900, Quantity: 10, CategoryID: categories[0].ID},
			{Name: "ASUS Vivobook", Price: 21900, Quantity: 15, CategoryID: categories[0].ID},
			{Name: "Lenovo IdeaPad", Price: 18900, Quantity: 8, CategoryID: categories[0].ID},
			{Name: "Acer Aspire", Price: 16900, Quantity: 4, CategoryID: categories[0].ID},
			{Name: "iPhone 16", Price: 29900, Quantity: 12, CategoryID: categories[1].ID},
			{Name: "Samsung Galaxy", Price: 19900, Quantity: 10, CategoryID: categories[1].ID},
			{Name: "Google Pixel", Price: 15900, Quantity: 3, CategoryID: categories[1].ID},
			{Name: "Logitech Mouse", Price: 590, Quantity: 30, CategoryID: categories[2].ID},
			{Name: "Mechanical Keyboard", Price: 2490, Quantity: 20, CategoryID: categories[2].ID},
			{Name: "USB-C Hub", Price: 1290, Quantity: 18, CategoryID: categories[2].ID},
			{Name: "Webcam Full HD", Price: 1590, Quantity: 4, CategoryID: categories[2].ID},
			{Name: "USB-C Cable", Price: 199.50, Quantity: 50, CategoryID: categories[2].ID},
			{Name: "Dell Monitor 24", Price: 4590, Quantity: 7, CategoryID: categories[3].ID},
			{Name: "Samsung SSD 1TB", Price: 3290, Quantity: 14, CategoryID: categories[3].ID},
			{Name: "External HDD 2TB", Price: 2490, Quantity: 2, CategoryID: categories[3].ID},
		}
		for i := range products {
			if err := createProductDB(tx, &products[i]); err != nil {
				return err
			}
		}
		customers := []Customer{
			{Name: "สมชาย ใจดี", Email: "somchai@example.com", Phone: "0812345678"},
			{Name: "สมหญิง รักเรียน", Email: "somying@example.com", Phone: "0823456789"},
			{Name: "อนันต์ มั่นคง", Email: "anan@example.com", Phone: "0834567890"},
			{Name: "มาลี สุขใจ", Email: "malee@example.com", Phone: "0845678901"},
			{Name: "วิชัย ก้าวหน้า", Email: "wichai@example.com", Phone: "0856789012"},
			{Name: "สุดา พัฒนา", Email: "suda@example.com", Phone: "0867890123"},
			{Name: "กิตติ มีทรัพย์", Email: "kitti@example.com", Phone: "0878901234"},
			{Name: "นิดา สดใส", Email: "nida@example.com", Phone: "0889012345"},
		}
		for i := range customers {
			if err := createCustomerDB(tx, &customers[i]); err != nil {
				return err
			}
		}
		// มีครบสามสถานะ และวันที่กระจายย้อนหลัง 12 วันจนถึงวันนี้
		entries := []struct {
			customer, product, quantity int
			status                      string
		}{
			{0, 0, 1, StatusCompleted}, {1, 1, 1, StatusPending},
			{2, 4, 1, StatusCompleted}, {3, 5, 2, StatusPending},
			{4, 7, 3, StatusCompleted}, {5, 8, 2, StatusCancelled},
			{6, 9, 1, StatusPending}, {7, 12, 1, StatusCompleted},
			{0, 13, 2, StatusPending}, {1, 11, 4, StatusCompleted},
			{2, 10, 1, StatusCancelled}, {3, 14, 1, StatusPending},
		}
		now := tx.NowFunc()
		for i, entry := range entries {
			date := now.AddDate(0, 0, i-11)
			date = time.Date(date.Year(), date.Month(), date.Day(), 10, 0, 0, 0, now.Location())
			order := Order{
				Model:      gorm.Model{CreatedAt: date, UpdatedAt: date},
				CustomerID: customers[entry.customer].ID,
				ProductID:  products[entry.product].ID, Quantity: entry.quantity,
			}
			if err := createOrderDB(tx, &order); err != nil {
				return err
			}
			if entry.status != StatusPending {
				if err := updateOrderStatusDB(tx, order.ID, entry.status); err != nil {
					return err
				}
			}
		}
		seeded = true
		return nil
	})
	return seeded && err == nil, err
}
