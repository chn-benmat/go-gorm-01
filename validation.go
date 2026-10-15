package main

import (
	"errors"
	"fmt"
	"math"
	"net/mail"
	"strings"
	"unicode/utf8"
)

var (
	ErrInvalidInput      = errors.New("ข้อมูลไม่ถูกต้อง")
	ErrInsufficientStock = errors.New("สต็อกสินค้าไม่เพียงพอ")
	ErrReferenced        = errors.New("ข้อมูลยังถูกใช้งานโดยรายการอื่น")
	ErrInvalidTransition = errors.New("ไม่สามารถเปลี่ยนสถานะนี้ได้")
)

const (
	maxStock      = 2_147_483_647 // จำกัดจำนวนในช่วง signed 32-bit
	maxPrice      = 9_999_999_999.99
	maxTotalPrice = 999_999_999_999.99
)

func validateID(id uint) error {
	if id == 0 {
		return fmt.Errorf("%w: ID ต้องมากกว่า 0", ErrInvalidInput)
	}
	return nil
}

func validateText(value, field string, maxLength int) error {
	if strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > maxLength {
		return fmt.Errorf("%w: %s ต้องมี 1-%d ตัวอักษร", ErrInvalidInput, field, maxLength)
	}
	return nil
}

func validatePrice(price float64) error {
	if math.IsNaN(price) || math.IsInf(price, 0) || price < 0 || price > maxPrice {
		return fmt.Errorf("%w: ราคาต้องอยู่ระหว่าง 0 ถึง %.2f", ErrInvalidInput, maxPrice)
	}
	return nil
}

func roundMoney(value float64) float64 {
	return math.Round(value*100) / 100
}

func validateQuantity(quantity int, allowZero bool) error {
	minimum := 1
	if allowZero {
		minimum = 0
	}
	if quantity < minimum || quantity > maxStock {
		return fmt.Errorf("%w: จำนวนต้องอยู่ระหว่าง %d ถึง %d", ErrInvalidInput, minimum, maxStock)
	}
	return nil
}

func validateProduct(product *Product) error {
	if product == nil {
		return fmt.Errorf("%w: ไม่มีข้อมูลสินค้า", ErrInvalidInput)
	}
	product.Name = strings.TrimSpace(product.Name)
	if err := validateText(product.Name, "ชื่อสินค้า", 150); err != nil {
		return err
	}
	if err := validatePrice(product.Price); err != nil {
		return err
	}
	if err := validateQuantity(product.Quantity, true); err != nil {
		return err
	}
	product.Price = roundMoney(product.Price)
	return validateID(product.CategoryID)
}

func validateCustomer(customer *Customer) error {
	if customer == nil {
		return fmt.Errorf("%w: ไม่มีข้อมูลลูกค้า", ErrInvalidInput)
	}
	customer.Name = strings.TrimSpace(customer.Name)
	customer.Email = strings.ToLower(strings.TrimSpace(customer.Email))
	customer.Phone = strings.TrimSpace(customer.Phone)
	for _, field := range []struct {
		value, name string
		max         int
	}{
		{customer.Name, "ชื่อลูกค้า", 150},
		{customer.Email, "อีเมล", 254},
		{customer.Phone, "เบอร์โทร", 30},
	} {
		if err := validateText(field.value, field.name, field.max); err != nil {
			return err
		}
	}
	address, err := mail.ParseAddress(customer.Email)
	if err != nil || address.Address != customer.Email {
		return fmt.Errorf("%w: รูปแบบอีเมลไม่ถูกต้อง", ErrInvalidInput)
	}
	return nil
}

func validatePriceRange(minPrice, maxPrice float64) error {
	if err := validatePrice(minPrice); err != nil {
		return err
	}
	if err := validatePrice(maxPrice); err != nil {
		return err
	}
	if minPrice > maxPrice {
		return fmt.Errorf("%w: ราคาต่ำสุดต้องไม่เกินราคาสูงสุด", ErrInvalidInput)
	}
	return nil
}

func validateStatus(status string) error {
	switch status {
	case StatusPending, StatusCompleted, StatusCancelled:
		return nil
	default:
		return fmt.Errorf("%w: สถานะต้องเป็น Pending, Completed หรือ Cancelled", ErrInvalidInput)
	}
}
