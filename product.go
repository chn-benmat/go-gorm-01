package main

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func createProductDB(db *gorm.DB, product *Product) error {
	if err := validateProduct(product); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var category Category
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&category, product.CategoryID).Error; err != nil {
			return fmt.Errorf("ประเภทสินค้า: %w", err)
		}
		return tx.Omit(clause.Associations).Create(product).Error
	})
}

func findProducts(db *gorm.DB) ([]Product, error) {
	var products []Product
	err := db.Preload("Category").Find(&products).Error
	return products, err
}

func getAllProductsDB(db *gorm.DB) ([]Product, error) {
	return findProducts(db.Order("id ASC"))
}

func getProductByIDDB(db *gorm.DB, id uint) (*Product, error) {
	if err := validateID(id); err != nil {
		return nil, err
	}
	var product Product
	err := db.Preload("Category").First(&product, id).Error
	return &product, err
}

func getProductByNameDB(db *gorm.DB, name string) ([]Product, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: กรุณาระบุชื่อบางส่วน", ErrInvalidInput)
	}
	return findProducts(db.Where("name LIKE ?", "%"+name+"%").Order("id ASC"))
}

func getProductsByCategoryDB(db *gorm.DB, categoryID uint) ([]Product, error) {
	if err := validateID(categoryID); err != nil {
		return nil, err
	}
	return findProducts(db.Where("category_id = ?", categoryID).Order("id ASC"))
}

func getProductsByPriceRangeDB(db *gorm.DB, minPrice, maxPrice float64) ([]Product, error) {
	if err := validatePriceRange(minPrice, maxPrice); err != nil {
		return nil, err
	}
	return findProducts(db.Where("price BETWEEN ? AND ?", minPrice, maxPrice).Order("price ASC, id ASC"))
}

func getProductsLowStockDB(db *gorm.DB, threshold int) ([]Product, error) {
	if err := validateQuantity(threshold, true); err != nil {
		return nil, err
	}
	return findProducts(db.Where("quantity < ?", threshold).Order("quantity ASC, id ASC"))
}

func getProductsByCategoryAndPriceDB(db *gorm.DB, categoryID uint, minPrice, maxPrice float64) ([]Product, error) {
	if err := validateID(categoryID); err != nil {
		return nil, err
	}
	if err := validatePriceRange(minPrice, maxPrice); err != nil {
		return nil, err
	}
	return findProducts(db.Where("category_id = ? AND price BETWEEN ? AND ?", categoryID, minPrice, maxPrice).Order("price ASC, id ASC"))
}

func getProductsSortedByPriceDB(db *gorm.DB, direction string) ([]Product, error) {
	// Order ไม่ใช้ placeholder จึงยอมรับเฉพาะค่าที่กำหนดไว้
	switch strings.ToUpper(strings.TrimSpace(direction)) {
	case "ASC":
		return findProducts(db.Order("price ASC, id ASC"))
	case "DESC":
		return findProducts(db.Order("price DESC, id ASC"))
	default:
		return nil, fmt.Errorf("%w: ลำดับราคาต้องเป็น ASC หรือ DESC", ErrInvalidInput)
	}
}

func getTopExpensiveProductsDB(db *gorm.DB, limit int) ([]Product, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%w: จำนวนรายการต้องมากกว่า 0", ErrInvalidInput)
	}
	return findProducts(db.Order("price DESC, id ASC").Limit(limit))
}

func updateProductByIDDB(db *gorm.DB, id uint, updates Product) error {
	if err := validateID(id); err != nil {
		return err
	}
	if err := validateProduct(&updates); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var category Category
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&category, updates.CategoryID).Error; err != nil {
			return fmt.Errorf("ประเภทสินค้า: %w", err)
		}
		var product Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, id).Error; err != nil {
			return err
		}
		// map ทำให้แก้ราคาและจำนวนเป็น 0 ได้ ต่างจาก Updates(struct)
		return tx.Model(&product).Updates(map[string]any{
			"name": updates.Name, "price": updates.Price,
			"quantity": updates.Quantity, "category_id": updates.CategoryID,
		}).Error
	})
}

func updateProductPriceDB(db *gorm.DB, id uint, price float64) error {
	if err := validateID(id); err != nil {
		return err
	}
	if err := validatePrice(price); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var product Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, id).Error; err != nil {
			return err
		}
		return tx.Model(&product).Update("price", roundMoney(price)).Error
	})
}

// delta เป็นจำนวนที่เพิ่ม/ลด เช่น +10 เติมสินค้า และ -3 ลดสินค้า
func updateProductStockDB(db *gorm.DB, id uint, delta int) error {
	if err := validateID(id); err != nil {
		return err
	}
	if delta < -maxStock || delta > maxStock {
		return fmt.Errorf("%w: จำนวนที่ปรับสต็อกเกินขอบเขต", ErrInvalidInput)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var product Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, id).Error; err != nil {
			return err
		}
		quantity := product.Quantity + delta
		if err := validateQuantity(quantity, true); err != nil {
			return err
		}
		return tx.Model(&product).Update("quantity", quantity).Error
	})
}

func softDeleteProductDB(db *gorm.DB, id uint) error {
	if err := validateID(id); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var product Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, id).Error; err != nil {
			return err
		}
		return tx.Delete(&product).Error
	})
}

func hardDeleteProductDB(db *gorm.DB, id uint) error {
	if err := validateID(id); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var product Product
		if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, id).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Unscoped().Model(&Order{}).Where("product_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: สินค้าอยู่ในประวัติคำสั่งซื้อ %d รายการ ใช้ Soft Delete แทนได้", ErrReferenced, count)
		}
		return tx.Unscoped().Delete(&product).Error
	})
}

func getDeletedProductsDB(db *gorm.DB) ([]Product, error) {
	return findProducts(db.Unscoped().Where("products.deleted_at IS NOT NULL").Order("id ASC"))
}

func restoreProductDB(db *gorm.DB, id uint) error {
	if err := validateID(id); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var product Product
		if err := tx.Unscoped().Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, id).Error; err != nil {
			return err
		}
		if !product.DeletedAt.Valid {
			return fmt.Errorf("%w: สินค้านี้ยังไม่ได้ถูก Soft Delete", ErrInvalidInput)
		}
		// ตรวจประเภทเดิมก่อนกู้คืน โดยไม่ต้องล็อกกลับลำดับกับ updateProduct
		var category Category
		if err := tx.First(&category, product.CategoryID).Error; err != nil {
			return fmt.Errorf("ประเภทสินค้าของรายการที่ต้องการกู้คืน: %w", err)
		}
		return tx.Unscoped().Model(&product).Update("deleted_at", nil).Error
	})
}
