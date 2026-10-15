package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"gorm.io/gorm"
)

type console struct {
	db    *gorm.DB
	input *bufio.Scanner
	out   io.Writer
}

type menuAction struct {
	label string
	run   func() error
}

var errConsoleInput = errors.New("อ่านข้อมูลจาก Console ไม่สำเร็จ")

func newConsole(db *gorm.DB, input io.Reader, output io.Writer) *console {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 1024), 64*1024)
	return &console{db: db, input: scanner, out: output}
}

func (c *console) text(label string) (string, error) {
	fmt.Fprint(c.out, label, ": ")
	if !c.input.Scan() {
		if err := c.input.Err(); err != nil {
			return "", fmt.Errorf("%w: %v", errConsoleInput, err)
		}
		return "", io.EOF
	}
	return strings.TrimSpace(c.input.Text()), nil
}

func (c *console) integer(label string) (int, error) {
	value, err := c.text(label)
	if err != nil {
		return 0, err
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%w: กรุณากรอกจำนวนเต็ม", ErrInvalidInput)
	}
	return number, nil
}

func (c *console) id(label string) (uint, error) {
	value, err := c.text(label)
	if err != nil {
		return 0, err
	}
	number, err := strconv.ParseUint(value, 10, strconv.IntSize)
	if err != nil || number == 0 {
		return 0, fmt.Errorf("%w: ID ต้องเป็นจำนวนเต็มมากกว่า 0", ErrInvalidInput)
	}
	return uint(number), nil
}

func (c *console) money(label string) (float64, error) {
	value, err := c.text(label)
	if err != nil {
		return 0, err
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: กรุณากรอกตัวเลข เช่น 1290.50", ErrInvalidInput)
	}
	return number, validatePrice(number)
}

func (c *console) priceRange() (float64, float64, error) {
	minimum, err := c.money("ราคาต่ำสุด")
	if err != nil {
		return 0, 0, err
	}
	maximum, err := c.money("ราคาสูงสุด")
	return minimum, maximum, err
}

func (c *console) date(label string) (time.Time, error) {
	value, err := c.text(label + " (YYYY-MM-DD)")
	if err != nil {
		return time.Time{}, err
	}
	date, err := time.ParseInLocation("2006-01-02", value, c.db.NowFunc().Location())
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: ใช้วันที่รูปแบบ YYYY-MM-DD", ErrInvalidInput)
	}
	return date, nil
}

func (c *console) menu(title string, actions []menuAction) error {
	for {
		fmt.Fprintln(c.out, "\n=== "+title+" ===")
		for i, action := range actions {
			fmt.Fprintf(c.out, "%d. %s\n", i+1, action.label)
		}
		fmt.Fprintln(c.out, "0. กลับ / ออก")
		choice, err := c.integer("เลือกเมนู")
		if errors.Is(err, io.EOF) || errors.Is(err, errConsoleInput) {
			return err
		}
		if err == nil && choice == 0 {
			return nil
		}
		if err == nil && (choice < 0 || choice > len(actions)) {
			err = fmt.Errorf("%w: ไม่มีเมนูนี้", ErrInvalidInput)
		}
		if err == nil {
			err = actions[choice-1].run()
		}
		if errors.Is(err, io.EOF) || errors.Is(err, errConsoleInput) {
			return err
		}
		if err != nil {
			switch {
			case errors.Is(err, gorm.ErrRecordNotFound):
				fmt.Fprintln(c.out, "ไม่พบข้อมูล หรือรายการถูกลบแล้ว")
			case errors.Is(err, gorm.ErrDuplicatedKey):
				fmt.Fprintln(c.out, "ข้อมูลซ้ำ เช่น อีเมลนี้ถูกใช้แล้ว")
			default:
				fmt.Fprintln(c.out, "ข้อผิดพลาด:", err)
			}
		}
	}
}

func (c *console) success(err error) error {
	if err == nil {
		fmt.Fprintln(c.out, "บันทึกข้อมูลสำเร็จ")
	}
	return err
}

func (c *console) run() error {
	return c.menu("ระบบจัดการร้านค้า Go + GORM", []menuAction{
		{"ประเภทสินค้า", c.categoryMenu},
		{"สินค้าและการค้นหา", c.productMenu},
		{"ลูกค้า", c.customerMenu},
		{"คำสั่งซื้อ", c.orderMenu},
		{"ตัวอย่างการค้นหาทั้ง 14 แบบ", c.demo},
	})
}

func (c *console) showCategories(categories []Category, err error) error {
	if err != nil {
		return err
	}
	if len(categories) == 0 {
		fmt.Fprintln(c.out, "ไม่พบประเภทสินค้า")
		return nil
	}
	w := tabwriter.NewWriter(c.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tประเภท\tรายละเอียด")
	for _, item := range categories {
		fmt.Fprintf(w, "%d\t%s\t%s\n", item.ID, item.Name, item.Description)
	}
	return w.Flush()
}

func (c *console) showProducts(products []Product, err error) error {
	if err != nil {
		return err
	}
	if len(products) == 0 {
		fmt.Fprintln(c.out, "ไม่พบสินค้า")
		return nil
	}
	w := tabwriter.NewWriter(c.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tสินค้า\tราคา (บาท)\tคงเหลือ\tCategoryID\tประเภท\tลบแล้ว")
	for _, item := range products {
		fmt.Fprintf(w, "%d\t%s\t%.2f\t%d\t%d\t%s\t%t\n", item.ID, item.Name, item.Price, item.Quantity, item.CategoryID, item.Category.Name, item.DeletedAt.Valid)
	}
	return w.Flush()
}

func (c *console) showCustomers(customers []Customer, err error) error {
	if err != nil {
		return err
	}
	if len(customers) == 0 {
		fmt.Fprintln(c.out, "ไม่พบลูกค้า")
		return nil
	}
	w := tabwriter.NewWriter(c.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tลูกค้า\tอีเมล\tเบอร์โทร")
	for _, item := range customers {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\n", item.ID, item.Name, item.Email, item.Phone)
	}
	return w.Flush()
}

func (c *console) showOrders(orders []Order, err error) error {
	if err != nil {
		return err
	}
	if len(orders) == 0 {
		fmt.Fprintln(c.out, "ไม่พบคำสั่งซื้อ")
		return nil
	}
	w := tabwriter.NewWriter(c.out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tวันที่\tลูกค้า (ID)\tสินค้า (ID)\tจำนวน\tยอดรวม (บาท)\tสถานะ")
	for _, item := range orders {
		fmt.Fprintf(w, "%d\t%s\t%s (%d)\t%s (%d)\t%d\t%.2f\t%s\n", item.ID, item.CreatedAt.Format("2006-01-02 15:04"), item.Customer.Name, item.CustomerID, item.Product.Name, item.ProductID, item.Quantity, item.TotalPrice, item.Status)
	}
	return w.Flush()
}

func (c *console) categoryMenu() error {
	return c.menu("ประเภทสินค้า", []menuAction{
		{"ดูทั้งหมด", func() error { return c.showCategories(getAllCategoriesDB(c.db)) }},
		{"เพิ่มประเภท", func() error {
			name, err := c.text("ชื่อประเภท")
			if err != nil {
				return err
			}
			description, err := c.text("รายละเอียด (เว้นว่างได้)")
			if err != nil {
				return err
			}
			item := Category{Name: name, Description: description}
			return c.success(createCategoryDB(c.db, &item))
		}},
		{"ลบประเภทที่ไม่มีสินค้า", func() error {
			id, err := c.id("Category ID")
			if err != nil {
				return err
			}
			return c.success(deleteCategoryByIDDB(c.db, id))
		}},
	})
}

func (c *console) productInput() (Product, error) {
	var item Product
	if err := c.showCategories(getAllCategoriesDB(c.db)); err != nil {
		return item, err
	}
	var err error
	item.Name, err = c.text("ชื่อสินค้า")
	if err != nil {
		return item, err
	}
	item.Price, err = c.money("ราคา")
	if err != nil {
		return item, err
	}
	item.Quantity, err = c.integer("จำนวนคงเหลือ")
	if err != nil {
		return item, err
	}
	item.CategoryID, err = c.id("Category ID")
	return item, err
}

func (c *console) productMenu() error {
	return c.menu("สินค้าและการค้นหา", []menuAction{
		{"สินค้าทั้งหมด", func() error { return c.showProducts(getAllProductsDB(c.db)) }},
		{"ค้นหาจาก ID", func() error {
			id, err := c.id("Product ID")
			if err != nil {
				return err
			}
			item, err := getProductByIDDB(c.db, id)
			if err != nil {
				return err
			}
			return c.showProducts([]Product{*item}, nil)
		}},
		{"ค้นหาชื่อบางส่วน (LIKE)", func() error {
			name, err := c.text("ชื่อบางส่วน")
			if err != nil {
				return err
			}
			return c.showProducts(getProductByNameDB(c.db, name))
		}},
		{"ค้นหาตามประเภท", func() error {
			id, err := c.id("Category ID")
			if err != nil {
				return err
			}
			return c.showProducts(getProductsByCategoryDB(c.db, id))
		}},
		{"ค้นหาตามช่วงราคา", func() error {
			minimum, maximum, err := c.priceRange()
			if err != nil {
				return err
			}
			return c.showProducts(getProductsByPriceRangeDB(c.db, minimum, maximum))
		}},
		{"สินค้าที่สต็อกน้อยกว่าค่าที่กำหนด", func() error {
			number, err := c.integer("สต็อกน้อยกว่า")
			if err != nil {
				return err
			}
			return c.showProducts(getProductsLowStockDB(c.db, number))
		}},
		{"ค้นหาประเภทและช่วงราคา", func() error {
			id, err := c.id("Category ID")
			if err != nil {
				return err
			}
			minimum, maximum, err := c.priceRange()
			if err != nil {
				return err
			}
			return c.showProducts(getProductsByCategoryAndPriceDB(c.db, id, minimum, maximum))
		}},
		{"เรียงราคา ASC/DESC", func() error {
			direction, err := c.text("ASC หรือ DESC")
			if err != nil {
				return err
			}
			return c.showProducts(getProductsSortedByPriceDB(c.db, direction))
		}},
		{"สินค้าที่แพงที่สุด N รายการ", func() error {
			limit, err := c.integer("จำนวนรายการ")
			if err != nil {
				return err
			}
			return c.showProducts(getTopExpensiveProductsDB(c.db, limit))
		}},
		{"เพิ่มสินค้า", func() error {
			item, err := c.productInput()
			if err != nil {
				return err
			}
			return c.success(createProductDB(c.db, &item))
		}},
		{"แก้ไขข้อมูลสินค้า (กรอกข้อมูลใหม่ทุกช่อง)", func() error {
			id, err := c.id("Product ID")
			if err != nil {
				return err
			}
			item, err := c.productInput()
			if err != nil {
				return err
			}
			return c.success(updateProductByIDDB(c.db, id, item))
		}},
		{"แก้ไขราคา", func() error {
			id, err := c.id("Product ID")
			if err != nil {
				return err
			}
			price, err := c.money("ราคาใหม่")
			if err != nil {
				return err
			}
			return c.success(updateProductPriceDB(c.db, id, price))
		}},
		{"เพิ่ม/ลดสต็อก", func() error {
			id, err := c.id("Product ID")
			if err != nil {
				return err
			}
			delta, err := c.integer("จำนวนที่เพิ่ม/ลด เช่น 10 หรือ -3")
			if err != nil {
				return err
			}
			return c.success(updateProductStockDB(c.db, id, delta))
		}},
		{"Soft Delete สินค้า", func() error {
			id, err := c.id("Product ID")
			if err != nil {
				return err
			}
			return c.success(softDeleteProductDB(c.db, id))
		}},
		{"ลบสินค้าถาวร (ห้ามมีประวัติคำสั่งซื้อ)", func() error {
			id, err := c.id("Product ID")
			if err != nil {
				return err
			}
			confirmation, err := c.text("พิมพ์ DELETE เพื่อยืนยันลบถาวร")
			if err != nil {
				return err
			}
			if confirmation != "DELETE" {
				fmt.Fprintln(c.out, "ยกเลิกการลบ")
				return nil
			}
			return c.success(hardDeleteProductDB(c.db, id))
		}},
		{"ดูสินค้าที่ถูก Soft Delete", func() error { return c.showProducts(getDeletedProductsDB(c.db)) }},
		{"กู้คืนสินค้า", func() error {
			id, err := c.id("Product ID")
			if err != nil {
				return err
			}
			return c.success(restoreProductDB(c.db, id))
		}},
	})
}

func (c *console) customerInput() (Customer, error) {
	var item Customer
	var err error
	item.Name, err = c.text("ชื่อลูกค้า")
	if err != nil {
		return item, err
	}
	item.Email, err = c.text("อีเมล")
	if err != nil {
		return item, err
	}
	item.Phone, err = c.text("เบอร์โทร")
	return item, err
}

func (c *console) customerMenu() error {
	return c.menu("ลูกค้า", []menuAction{
		{"ดูทั้งหมด", func() error { return c.showCustomers(getAllCustomersDB(c.db)) }},
		{"ค้นหาด้วยอีเมล", func() error {
			email, err := c.text("อีเมล")
			if err != nil {
				return err
			}
			item, err := getCustomerByEmailDB(c.db, email)
			if err != nil {
				return err
			}
			return c.showCustomers([]Customer{*item}, nil)
		}},
		{"เพิ่มลูกค้า", func() error {
			item, err := c.customerInput()
			if err != nil {
				return err
			}
			return c.success(createCustomerDB(c.db, &item))
		}},
		{"แก้ไขลูกค้า (กรอกข้อมูลใหม่ทุกช่อง)", func() error {
			id, err := c.id("Customer ID")
			if err != nil {
				return err
			}
			item, err := c.customerInput()
			if err != nil {
				return err
			}
			return c.success(updateCustomerByIDDB(c.db, id, item))
		}},
		{"ลบลูกค้าที่ไม่มีประวัติคำสั่งซื้อ", func() error {
			id, err := c.id("Customer ID")
			if err != nil {
				return err
			}
			return c.success(deleteCustomerByIDDB(c.db, id))
		}},
	})
}

func (c *console) orderMenu() error {
	return c.menu("คำสั่งซื้อ", []menuAction{
		{"ดูทั้งหมดพร้อมลูกค้าและสินค้า (Preload)", func() error { return c.showOrders(getOrdersWithDetailsDB(c.db)) }},
		{"ค้นหาตามลูกค้า", func() error {
			id, err := c.id("Customer ID")
			if err != nil {
				return err
			}
			return c.showOrders(getOrdersByCustomerDB(c.db, id))
		}},
		{"ค้นหาตามสถานะ", func() error {
			status, err := c.text("Pending / Completed / Cancelled")
			if err != nil {
				return err
			}
			return c.showOrders(getOrdersByStatusDB(c.db, status))
		}},
		{"ค้นหาตามช่วงวันที่ (รวมวันแรกและวันสุดท้าย)", func() error {
			start, err := c.date("วันเริ่มต้น")
			if err != nil {
				return err
			}
			end, err := c.date("วันสิ้นสุด")
			if err != nil {
				return err
			}
			return c.showOrders(getOrdersByDateRangeDB(c.db, start, end))
		}},
		{"สร้างคำสั่งซื้อ (Pending)", func() error {
			if err := c.showCustomers(getAllCustomersDB(c.db)); err != nil {
				return err
			}
			if err := c.showProducts(getAllProductsDB(c.db)); err != nil {
				return err
			}
			customerID, err := c.id("Customer ID")
			if err != nil {
				return err
			}
			productID, err := c.id("Product ID")
			if err != nil {
				return err
			}
			quantity, err := c.integer("จำนวนที่สั่ง")
			if err != nil {
				return err
			}
			item := Order{CustomerID: customerID, ProductID: productID, Quantity: quantity}
			if err := createOrderDB(c.db, &item); err != nil {
				return err
			}
			fmt.Fprintf(c.out, "สร้างคำสั่งซื้อ #%d ยอดรวม %.2f บาท สถานะ %s\n", item.ID, item.TotalPrice, item.Status)
			return nil
		}},
		{"เปลี่ยนสถานะ Pending -> Completed/Cancelled", func() error {
			id, err := c.id("Order ID")
			if err != nil {
				return err
			}
			status, err := c.text("สถานะใหม่ (Completed / Cancelled)")
			if err != nil {
				return err
			}
			return c.success(updateOrderStatusDB(c.db, id, status))
		}},
	})
}

func (c *console) demo() error {
	products, err := getAllProductsDB(c.db)
	if err != nil {
		return err
	}
	customers, err := getAllCustomersDB(c.db)
	if err != nil {
		return err
	}
	if len(products) == 0 || len(customers) == 0 {
		fmt.Fprintln(c.out, "ต้องมีสินค้าและลูกค้าก่อนแสดงตัวอย่าง")
		return nil
	}
	product, customer := products[0], customers[0]
	queries := []menuAction{
		{"getAllProductsDB", func() error { return c.showProducts(products, nil) }},
		{"getProductByIDDB", func() error {
			item, err := getProductByIDDB(c.db, product.ID)
			if err != nil {
				return err
			}
			return c.showProducts([]Product{*item}, nil)
		}},
		{"getProductByNameDB (Mac)", func() error { return c.showProducts(getProductByNameDB(c.db, "Mac")) }},
		{"getProductsByCategoryDB", func() error { return c.showProducts(getProductsByCategoryDB(c.db, product.CategoryID)) }},
		{"getProductsByPriceRangeDB (1000-10000)", func() error { return c.showProducts(getProductsByPriceRangeDB(c.db, 1000, 10000)) }},
		{"getProductsLowStockDB (< 5)", func() error { return c.showProducts(getProductsLowStockDB(c.db, 5)) }},
		{"getProductsByCategoryAndPriceDB", func() error {
			return c.showProducts(getProductsByCategoryAndPriceDB(c.db, product.CategoryID, 500, 50000))
		}},
		{"getProductsSortedByPriceDB (ASC)", func() error { return c.showProducts(getProductsSortedByPriceDB(c.db, "ASC")) }},
		{"getTopExpensiveProductsDB (3)", func() error { return c.showProducts(getTopExpensiveProductsDB(c.db, 3)) }},
		{"getCustomerByEmailDB", func() error {
			item, err := getCustomerByEmailDB(c.db, customer.Email)
			if err != nil {
				return err
			}
			return c.showCustomers([]Customer{*item}, nil)
		}},
		{"getOrdersByCustomerDB", func() error { return c.showOrders(getOrdersByCustomerDB(c.db, customer.ID)) }},
		{"getOrdersByStatusDB (Pending)", func() error { return c.showOrders(getOrdersByStatusDB(c.db, StatusPending)) }},
		{"getOrdersByDateRangeDB (30 วันย้อนหลัง)", func() error {
			now := c.db.NowFunc()
			return c.showOrders(getOrdersByDateRangeDB(c.db, now.AddDate(0, 0, -30), now))
		}},
		{"getOrdersWithDetailsDB", func() error { return c.showOrders(getOrdersWithDetailsDB(c.db)) }},
	}
	for i, query := range queries {
		fmt.Fprintf(c.out, "\n--- %d. %s ---\n", i+1, query.label)
		if err := query.run(); err != nil {
			return err
		}
	}
	return nil
}
