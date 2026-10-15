package main

import (
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func createOrderDB(db *gorm.DB, order *Order) error {
	if order == nil {
		return fmt.Errorf("%w: ไม่มีข้อมูลคำสั่งซื้อ", ErrInvalidInput)
	}
	if err := validateID(order.CustomerID); err != nil {
		return err
	}
	if err := validateID(order.ProductID); err != nil {
		return err
	}
	if err := validateQuantity(order.Quantity, false); err != nil {
		return err
	}
	if order.Status != "" && order.Status != StatusPending {
		return fmt.Errorf("%w: คำสั่งซื้อใหม่ต้องเป็น Pending", ErrInvalidInput)
	}
	// ใช้สำเนาเพื่อไม่ให้ผู้เรียกได้รับ ID ของคำสั่งซื้อที่ถูก rollback
	created := *order
	err := db.Transaction(func(tx *gorm.DB) error {
		var customer Customer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&customer, order.CustomerID).Error; err != nil {
			return fmt.Errorf("ลูกค้า: %w", err)
		}
		var product Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, order.ProductID).Error; err != nil {
			return fmt.Errorf("สินค้า: %w", err)
		}
		if product.Quantity < order.Quantity {
			return fmt.Errorf("%w: คงเหลือ %d ต้องการ %d", ErrInsufficientStock, product.Quantity, order.Quantity)
		}
		created.TotalPrice = roundMoney(product.Price * float64(order.Quantity))
		if created.TotalPrice > maxTotalPrice {
			return fmt.Errorf("%w: ยอดรวมเกินขอบเขตฐานข้อมูล", ErrInvalidInput)
		}
		created.Status = StatusPending
		// หากขั้นตอนใดล้มเหลว Transaction จะคืนทั้งสต็อกและคำสั่งซื้อ
		if err := tx.Model(&product).Update("quantity", gorm.Expr("quantity - ?", order.Quantity)).Error; err != nil {
			return err
		}
		return tx.Omit(clause.Associations).Create(&created).Error
	})
	if err == nil {
		*order = created
	}
	return err
}

func findOrders(db *gorm.DB) ([]Order, error) {
	var orders []Order
	err := db.Preload("Customer", func(tx *gorm.DB) *gorm.DB { return tx.Unscoped() }).
		Preload("Product", func(tx *gorm.DB) *gorm.DB { return tx.Unscoped() }).
		Order("orders.id ASC").Find(&orders).Error
	return orders, err
}

func getOrdersByCustomerDB(db *gorm.DB, customerID uint) ([]Order, error) {
	if err := validateID(customerID); err != nil {
		return nil, err
	}
	return findOrders(db.Where("customer_id = ?", customerID))
}

func getOrdersByStatusDB(db *gorm.DB, status string) ([]Order, error) {
	if err := validateStatus(status); err != nil {
		return nil, err
	}
	return findOrders(db.Where("status = ?", status))
}

// ช่วงวันที่รวมวันแรกและวันสุดท้าย โดยตัดเวลาตาม timezone Asia/Bangkok
func getOrdersByDateRangeDB(db *gorm.DB, start, end time.Time) ([]Order, error) {
	location := db.NowFunc().Location()
	start, end = start.In(location), end.In(location)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, location)
	end = time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, location)
	if start.After(end) {
		return nil, fmt.Errorf("%w: วันเริ่มต้นต้องไม่เกินวันสิ้นสุด", ErrInvalidInput)
	}
	return findOrders(db.Where("created_at >= ? AND created_at < ?", start, end.AddDate(0, 0, 1)))
}

func getOrdersWithDetailsDB(db *gorm.DB) ([]Order, error) {
	return findOrders(db)
}

func updateOrderStatusDB(db *gorm.DB, id uint, status string) error {
	if err := validateID(id); err != nil {
		return err
	}
	if err := validateStatus(status); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var order Order
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, id).Error; err != nil {
			return err
		}
		// เรียกสถานะเดิมซ้ำได้ แต่ต้องไม่คืนสต็อกซ้ำ
		if order.Status == status {
			return nil
		}
		if order.Status != StatusPending || status == StatusPending {
			return fmt.Errorf("%w: %s -> %s (อนุญาต Pending -> Completed/Cancelled เท่านั้น)", ErrInvalidTransition, order.Status, status)
		}
		if status == StatusCancelled {
			var product Product
			// ยังคืนสต็อกได้แม้สินค้าในคำสั่งซื้อถูก Soft Delete
			if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, order.ProductID).Error; err != nil {
				return err
			}
			quantity := product.Quantity + order.Quantity
			if err := validateQuantity(quantity, true); err != nil {
				return err
			}
			if err := tx.Unscoped().Model(&product).Update("quantity", quantity).Error; err != nil {
				return err
			}
		}
		return tx.Model(&order).Update("status", status).Error
	})
}
