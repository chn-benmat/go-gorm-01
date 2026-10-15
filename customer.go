package main

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func createCustomerDB(db *gorm.DB, customer *Customer) error {
	if err := validateCustomer(customer); err != nil {
		return err
	}
	return db.Omit(clause.Associations).Create(customer).Error
}

func getAllCustomersDB(db *gorm.DB) ([]Customer, error) {
	var customers []Customer
	err := db.Order("id ASC").Find(&customers).Error
	return customers, err
}

func getCustomerByEmailDB(db *gorm.DB, email string) (*Customer, error) {
	var customer Customer
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return nil, fmt.Errorf("%w: กรุณาระบุอีเมล", ErrInvalidInput)
	}
	err := db.Where("email = ?", email).First(&customer).Error
	return &customer, err
}

func updateCustomerByIDDB(db *gorm.DB, id uint, updates Customer) error {
	if err := validateID(id); err != nil {
		return err
	}
	if err := validateCustomer(&updates); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var customer Customer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&customer, id).Error; err != nil {
			return err
		}
		return tx.Model(&customer).Updates(map[string]any{
			"name": updates.Name, "email": updates.Email, "phone": updates.Phone,
		}).Error
	})
}

func deleteCustomerByIDDB(db *gorm.DB, id uint) error {
	if err := validateID(id); err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var customer Customer
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&customer, id).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Unscoped().Model(&Order{}).Where("customer_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: ลูกค้ามีประวัติคำสั่งซื้อ %d รายการ", ErrReferenced, count)
		}
		return tx.Delete(&customer).Error
	})
}
