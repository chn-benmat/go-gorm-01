package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func createCategoryDB(db *gorm.DB, category *Category) error {
	if category == nil {
		return fmt.Errorf("%w: ไม่มีข้อมูลประเภทสินค้า", ErrInvalidInput)
	}
	category.Name = strings.TrimSpace(category.Name)
	category.Description = strings.TrimSpace(category.Description)
	if err := validateText(category.Name, "ชื่อประเภท", 150); err != nil {
		return err
	}
	if utf8.RuneCountInString(category.Description) > 500 {
		return fmt.Errorf("%w: รายละเอียดต้องไม่เกิน 500 ตัวอักษร", ErrInvalidInput)
	}
	return db.Omit(clause.Associations).Create(category).Error
}

func getAllCategoriesDB(db *gorm.DB) ([]Category, error) {
	var categories []Category
	err := db.Order("id ASC").Find(&categories).Error
	return categories, err
}

func deleteCategoryByIDDB(db *gorm.DB, id uint) error {
	if err := validateID(id); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var category Category
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&category, id).Error; err != nil {
			return err
		}
		// รวมสินค้าที่ Soft Delete แล้ว เพื่อให้สินค้านั้นยังสามารถกู้คืนได้
		var count int64
		if err := tx.Unscoped().Model(&Product{}).Where("category_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: ประเภทนี้ยังมีสินค้า %d รายการ (รวม Soft Delete)", ErrReferenced, count)
		}
		return tx.Delete(&category).Error
	})
}
