package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ใช้ MySQL จริงในฐานข้อมูลชั่วคราวของแต่ละรอบ ไม่แตะ store_db
func newTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	config := loadDatabaseConfig()
	config.Name = fmt.Sprintf("store_test_%d", time.Now().UnixNano())
	db, err := openDatabase(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err != nil {
			t.Error(err)
			return
		}
		defer sqlDB.Close()
		// ชื่อนี้สร้างโดย test เท่านั้น และต้องมี prefix ของฐานข้อมูลทดสอบ
		if !strings.HasPrefix(config.Name, "store_test_") {
			t.Fatal("refusing to drop a non-test database")
		}
		if _, err := sqlDB.Exec("DROP DATABASE `" + config.Name + "`"); err != nil {
			t.Error(err)
		}
	})
	return db
}

func mustOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

var fixtureSequence atomic.Uint64

func fixture(t *testing.T, db *gorm.DB) (Category, Product, Customer) {
	t.Helper()
	n := fixtureSequence.Add(1)
	category := Category{Name: fmt.Sprintf("ประเภททดสอบ %d", n)}
	mustOK(t, createCategoryDB(db, &category))
	product := Product{Name: fmt.Sprintf("สินค้าทดสอบ %d", n), Price: 12.50, Quantity: 10, CategoryID: category.ID}
	mustOK(t, createProductDB(db, &product))
	customer := Customer{Name: "ลูกค้าทดสอบ", Email: fmt.Sprintf("customer-%d@example.com", n), Phone: "0812345678"}
	mustOK(t, createCustomerDB(db, &customer))
	return category, product, customer
}

func assertStock(t *testing.T, db *gorm.DB, id uint, expected int) {
	t.Helper()
	var product Product
	mustOK(t, db.Unscoped().First(&product, id).Error)
	if product.Quantity != expected {
		t.Fatalf("stock = %d, want %d", product.Quantity, expected)
	}
}

func containsProduct(products []Product, id uint) bool {
	for _, product := range products {
		if product.ID == id {
			return true
		}
	}
	return false
}

func TestMySQLIntegration(t *testing.T) {
	if os.Getenv("RUN_MYSQL_TESTS") != "1" {
		t.Skip("set RUN_MYSQL_TESTS=1 to test against MySQL/XAMPP")
	}
	db := newTestDatabase(t)

	t.Run("OrderAtomicityAndRollback", func(t *testing.T) {
		_, product, customer := fixture(t, db)
		order := Order{CustomerID: customer.ID, ProductID: product.ID, Quantity: 2, TotalPrice: 99999}
		mustOK(t, createOrderDB(db, &order))
		if order.ID == 0 || order.TotalPrice != 25 || order.Status != StatusPending {
			t.Fatalf("unexpected order: %+v", order)
		}
		assertStock(t, db, product.ID, 8)
		tooMany := Order{CustomerID: customer.ID, ProductID: product.ID, Quantity: 9}
		if err := createOrderDB(db, &tooMany); !errors.Is(err, ErrInsufficientStock) {
			t.Fatalf("insufficient stock: %v", err)
		}
		if tooMany.ID != 0 {
			t.Fatal("failed order received an ID")
		}
		// บังคับ INSERT ล้มเหลวหลัง UPDATE สต็อก ด้วย primary key ซ้ำ
		duplicate := Order{Model: gorm.Model{ID: order.ID}, CustomerID: customer.ID, ProductID: product.ID, Quantity: 1}
		if err := createOrderDB(db, &duplicate); !errors.Is(err, gorm.ErrDuplicatedKey) {
			t.Fatalf("expected duplicate insert failure: %v", err)
		}
		assertStock(t, db, product.ID, 8)
		orders, err := getOrdersByCustomerDB(db, customer.ID)
		mustOK(t, err)
		if len(orders) != 1 {
			t.Fatalf("orders = %d, want 1", len(orders))
		}
		for _, invalid := range []Order{
			{CustomerID: 99999999, ProductID: product.ID, Quantity: 1},
			{CustomerID: customer.ID, ProductID: 99999999, Quantity: 1},
		} {
			if err := createOrderDB(db, &invalid); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("missing relationship: %v", err)
			}
		}
		assertStock(t, db, product.ID, 8)
	})

	t.Run("CancellationSoftDeleteAndHistory", func(t *testing.T) {
		category, product, customer := fixture(t, db)
		order := Order{CustomerID: customer.ID, ProductID: product.ID, Quantity: 2}
		mustOK(t, createOrderDB(db, &order))
		mustOK(t, softDeleteProductDB(db, product.ID))
		if _, err := getProductByIDDB(db, product.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("soft deleted product visible: %v", err)
		}
		orders, err := getOrdersByCustomerDB(db, customer.ID)
		mustOK(t, err)
		if len(orders) != 1 || orders[0].Product.Name != product.Name || !orders[0].Product.DeletedAt.Valid || orders[0].Customer.Name != customer.Name {
			t.Fatalf("history lost its details: %+v", orders)
		}
		for _, operation := range []func() error{
			func() error { return hardDeleteProductDB(db, product.ID) },
			func() error { return deleteCategoryByIDDB(db, category.ID) },
			func() error { return deleteCustomerByIDDB(db, customer.ID) },
		} {
			if err := operation(); !errors.Is(err, ErrReferenced) {
				t.Fatalf("relationship guard: %v", err)
			}
		}
		blocked := Order{CustomerID: customer.ID, ProductID: product.ID, Quantity: 1}
		if err := createOrderDB(db, &blocked); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("ordered a deleted product: %v", err)
		}
		mustOK(t, updateOrderStatusDB(db, order.ID, StatusCancelled))
		mustOK(t, updateOrderStatusDB(db, order.ID, StatusCancelled))
		assertStock(t, db, product.ID, 10)
		if err := updateOrderStatusDB(db, order.ID, StatusCompleted); !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("reopened cancelled order: %v", err)
		}
		mustOK(t, restoreProductDB(db, product.ID))
		restored, err := getProductByIDDB(db, product.ID)
		mustOK(t, err)
		if restored.DeletedAt.Valid || restored.Quantity != 10 {
			t.Fatalf("bad restore: %+v", restored)
		}
		if err := hardDeleteProductDB(db, product.ID); !errors.Is(err, ErrReferenced) {
			t.Fatalf("cancelled history should block hard delete: %v", err)
		}
	})

	t.Run("CompletedIsFinalAndTotalIsHistorical", func(t *testing.T) {
		_, product, customer := fixture(t, db)
		order := Order{CustomerID: customer.ID, ProductID: product.ID, Quantity: 2}
		mustOK(t, createOrderDB(db, &order))
		mustOK(t, updateProductPriceDB(db, product.ID, 99))
		mustOK(t, updateOrderStatusDB(db, order.ID, StatusCompleted))
		mustOK(t, updateOrderStatusDB(db, order.ID, StatusCompleted))
		for _, status := range []string{StatusPending, StatusCancelled} {
			if err := updateOrderStatusDB(db, order.ID, status); !errors.Is(err, ErrInvalidTransition) {
				t.Fatalf("completed -> %s: %v", status, err)
			}
		}
		assertStock(t, db, product.ID, 8)
		var saved Order
		mustOK(t, db.First(&saved, order.ID).Error)
		if saved.TotalPrice != 25 {
			t.Fatalf("historical total changed: %v", saved.TotalPrice)
		}
	})

	t.Run("ValidationZeroUpdatesAndForeignKeys", func(t *testing.T) {
		category, product, customer := fixture(t, db)
		for _, operation := range []func() error{
			func() error { return updateProductPriceDB(db, product.ID, -1) },
			func() error { return updateProductStockDB(db, product.ID, -11) },
			func() error { return updateOrderStatusDB(db, 1, "Unknown") },
			func() error {
				item := Order{CustomerID: customer.ID, ProductID: product.ID, Quantity: 0}
				return createOrderDB(db, &item)
			},
			func() error {
				item := Product{Name: "invalid", Price: 1, Quantity: -1, CategoryID: category.ID}
				return createProductDB(db, &item)
			},
			func() error {
				item := Customer{Name: "invalid", Email: "bad-email", Phone: "0812345678"}
				return createCustomerDB(db, &item)
			},
		} {
			if err := operation(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected validation error: %v", err)
			}
		}
		assertStock(t, db, product.ID, 10)
		mustOK(t, updateProductByIDDB(db, product.ID, Product{Name: "สินค้าราคาเป็นศูนย์", Price: 0, Quantity: 0, CategoryID: category.ID}))
		saved, err := getProductByIDDB(db, product.ID)
		mustOK(t, err)
		if saved.Price != 0 || saved.Quantity != 0 {
			t.Fatalf("zero updates ignored: %+v", saved)
		}
		mustOK(t, updateProductStockDB(db, product.ID, 5))
		mustOK(t, updateProductStockDB(db, product.ID, -2))
		assertStock(t, db, product.ID, 3)
		mustOK(t, updateCustomerByIDDB(db, customer.ID, Customer{Name: "ชื่อใหม่", Email: "changed@example.com", Phone: "0899999999"}))
		updatedCustomer, err := getCustomerByEmailDB(db, "CHANGED@EXAMPLE.COM")
		mustOK(t, err)
		if updatedCustomer.ID != customer.ID || updatedCustomer.Name != "ชื่อใหม่" {
			t.Fatal("customer update failed")
		}
		duplicate := Customer{Name: "ซ้ำ", Email: "changed@example.com", Phone: "0812345678"}
		if err := createCustomerDB(db, &duplicate); !errors.Is(err, gorm.ErrDuplicatedKey) {
			t.Fatalf("duplicate email accepted: %v", err)
		}
		orphan := Product{Name: "orphan", Price: 1, Quantity: 1, CategoryID: 99999999}
		if err := db.Omit(clause.Associations).Create(&orphan).Error; !errors.Is(err, gorm.ErrForeignKeyViolated) {
			t.Fatalf("missing physical foreign key: %v", err)
		}
		if err := db.Model(&Product{}).Where("id = ?", product.ID).Update("quantity", -1).Error; err == nil {
			t.Fatal("database accepted negative stock")
		}
		assertStock(t, db, product.ID, 3)
	})

	t.Run("DeleteAndRestoreUnreferencedRecords", func(t *testing.T) {
		category, product, customer := fixture(t, db)
		mustOK(t, softDeleteProductDB(db, product.ID))
		deleted, err := getDeletedProductsDB(db)
		mustOK(t, err)
		if !containsProduct(deleted, product.ID) {
			t.Fatal("deleted product not listed")
		}
		active, err := getAllProductsDB(db)
		mustOK(t, err)
		if containsProduct(active, product.ID) {
			t.Fatal("soft deleted product listed as active")
		}
		mustOK(t, restoreProductDB(db, product.ID))
		mustOK(t, softDeleteProductDB(db, product.ID))
		mustOK(t, hardDeleteProductDB(db, product.ID))
		var missing Product
		if err := db.Unscoped().First(&missing, product.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("hard delete failed: %v", err)
		}
		mustOK(t, deleteCategoryByIDDB(db, category.ID))
		mustOK(t, deleteCustomerByIDDB(db, customer.ID))
		if err := createProductDB(db, &Product{Name: "deleted category", Price: 1, Quantity: 1, CategoryID: category.ID}); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("deleted category accepted: %v", err)
		}
		if _, err := getCustomerByEmailDB(db, customer.Email); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("deleted customer found: %v", err)
		}
	})

	t.Run("ProductQueries", func(t *testing.T) {
		category, base, _ := fixture(t, db)
		mustOK(t, updateProductByIDDB(db, base.ID, Product{Name: "QueryAlpha", Price: 10, Quantity: 0, CategoryID: category.ID}))
		second := Product{Name: "QueryBeta", Price: 20, Quantity: 5, CategoryID: category.ID}
		third := Product{Name: "QueryGamma", Price: 30, Quantity: 10, CategoryID: category.ID}
		mustOK(t, createProductDB(db, &second))
		mustOK(t, createProductDB(db, &third))
		all, err := getAllProductsDB(db)
		mustOK(t, err)
		if !containsProduct(all, base.ID) || !containsProduct(all, third.ID) {
			t.Fatal("all products missing fixtures")
		}
		byName, err := getProductByNameDB(db, "eryBet")
		mustOK(t, err)
		if len(byName) != 1 || byName[0].ID != second.ID {
			t.Fatal("LIKE query failed")
		}
		byCategory, err := getProductsByCategoryDB(db, category.ID)
		mustOK(t, err)
		if len(byCategory) != 3 {
			t.Fatalf("category query: %d", len(byCategory))
		}
		byPrice, err := getProductsByPriceRangeDB(db, 10, 20)
		mustOK(t, err)
		if !containsProduct(byPrice, base.ID) || !containsProduct(byPrice, second.ID) || containsProduct(byPrice, third.ID) {
			t.Fatal("price boundaries failed")
		}
		for _, item := range byPrice {
			if item.Price < 10 || item.Price > 20 {
				t.Fatal("price outside range")
			}
		}
		low, err := getProductsLowStockDB(db, 5)
		mustOK(t, err)
		if !containsProduct(low, base.ID) || containsProduct(low, second.ID) {
			t.Fatal("low stock threshold is not exclusive")
		}
		combined, err := getProductsByCategoryAndPriceDB(db, category.ID, 10, 20)
		mustOK(t, err)
		if len(combined) != 2 {
			t.Fatalf("combined query: %d", len(combined))
		}
		for _, item := range combined {
			if item.CategoryID != category.ID || item.Category.Name != category.Name {
				t.Fatal("category preload failed")
			}
		}
		for _, direction := range []string{"ASC", "DESC"} {
			sorted, err := getProductsSortedByPriceDB(db, direction)
			mustOK(t, err)
			for i := 1; i < len(sorted); i++ {
				if (direction == "ASC" && sorted[i-1].Price > sorted[i].Price) || (direction == "DESC" && sorted[i-1].Price < sorted[i].Price) {
					t.Fatal("wrong price ordering")
				}
			}
		}
		top, err := getTopExpensiveProductsDB(db, 3)
		mustOK(t, err)
		descending, err := getProductsSortedByPriceDB(db, "DESC")
		mustOK(t, err)
		if len(top) != 3 {
			t.Fatal("top limit failed")
		}
		for i := range top {
			if top[i].ID != descending[i].ID {
				t.Fatal("wrong top products")
			}
		}
		if _, err := getProductsSortedByPriceDB(db, "DESC; DROP TABLE products"); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid ordering accepted: %v", err)
		}
		if _, err := getProductsByPriceRangeDB(db, 20, 10); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("inverted range accepted")
		}
		if _, err := getTopExpensiveProductsDB(db, 0); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("zero top limit accepted")
		}
	})

	t.Run("OrderDateBoundariesAndQueries", func(t *testing.T) {
		_, product, customer := fixture(t, db)
		day := time.Date(2001, 2, 5, 0, 0, 0, 0, db.NowFunc().Location())
		for _, timestamp := range []time.Time{day, day.Add(24*time.Hour - time.Millisecond), day.AddDate(0, 0, 1)} {
			order := Order{Model: gorm.Model{CreatedAt: timestamp}, CustomerID: customer.ID, ProductID: product.ID, Quantity: 1}
			mustOK(t, createOrderDB(db, &order))
		}
		orders, err := getOrdersByDateRangeDB(db, day, day)
		mustOK(t, err)
		if len(orders) != 2 {
			t.Fatalf("inclusive date boundaries returned %d, want 2", len(orders))
		}
		byCustomer, err := getOrdersByCustomerDB(db, customer.ID)
		mustOK(t, err)
		if len(byCustomer) != 3 {
			t.Fatal("customer order history failed")
		}
		pending, err := getOrdersByStatusDB(db, StatusPending)
		mustOK(t, err)
		for _, order := range pending {
			if order.Status != StatusPending {
				t.Fatal("wrong order status")
			}
		}
		details, err := getOrdersWithDetailsDB(db)
		mustOK(t, err)
		if len(details) < 3 {
			t.Fatal("missing order details")
		}
		for _, order := range details {
			if order.Product.ID != order.ProductID || order.Customer.ID != order.CustomerID {
				t.Fatal("missing preloaded order relationship")
			}
		}
		if _, err := getOrdersByDateRangeDB(db, day.AddDate(0, 0, 1), day); !errors.Is(err, ErrInvalidInput) {
			t.Fatal("inverted dates accepted")
		}
	})

	t.Run("ConcurrentOrdersAndCancellation", func(t *testing.T) {
		_, product, customer := fixture(t, db)
		other := Customer{Name: "ผู้ซื้ออีกคน", Email: "concurrent@example.com", Phone: "0822222222"}
		mustOK(t, createCustomerDB(db, &other))
		mustOK(t, updateProductStockDB(db, product.ID, -9))
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for _, customerID := range []uint{customer.ID, other.ID} {
			wg.Add(1)
			go func(id uint) {
				defer wg.Done()
				<-start
				order := Order{CustomerID: id, ProductID: product.ID, Quantity: 1}
				results <- createOrderDB(db, &order)
			}(customerID)
		}
		close(start)
		wg.Wait()
		close(results)
		successes, rejected := 0, 0
		for err := range results {
			if err == nil {
				successes++
			} else if errors.Is(err, ErrInsufficientStock) {
				rejected++
			} else {
				t.Fatal(err)
			}
		}
		if successes != 1 || rejected != 1 {
			t.Fatalf("success=%d rejected=%d", successes, rejected)
		}
		assertStock(t, db, product.ID, 0)
		var orders []Order
		mustOK(t, db.Where("product_id = ?", product.ID).Find(&orders).Error)
		if len(orders) != 1 {
			t.Fatalf("oversold: %d orders", len(orders))
		}
		cancelStart := make(chan struct{})
		cancelResults := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-cancelStart
				cancelResults <- updateOrderStatusDB(db, orders[0].ID, StatusCancelled)
			}()
		}
		close(cancelStart)
		wg.Wait()
		close(cancelResults)
		for err := range cancelResults {
			mustOK(t, err)
		}
		assertStock(t, db, product.ID, 1)
	})

	t.Run("ConsoleOrderFlow", func(t *testing.T) {
		_, product, customer := fixture(t, db)
		var output bytes.Buffer
		// เมนูคำสั่งซื้อ -> สร้าง -> กลับ -> ออก
		input := fmt.Sprintf("4\n5\n%d\n%d\n2\n0\n0\n", customer.ID, product.ID)
		mustOK(t, newConsole(db, strings.NewReader(input), &output).run())
		orders, err := getOrdersByCustomerDB(db, customer.ID)
		mustOK(t, err)
		if len(orders) != 1 || orders[0].TotalPrice != 25 || !strings.Contains(output.String(), "สร้างคำสั่งซื้อ #") {
			t.Fatalf("console order creation failed: %s", output.String())
		}
		assertStock(t, db, product.ID, 8)
		// เมนูสินค้า -> Soft Delete; เมนูคำสั่งซื้อ -> ประวัติ -> ยกเลิก
		output.Reset()
		input = fmt.Sprintf("2\n14\n%d\n0\n4\n2\n%d\n6\n%d\nCancelled\n0\n0\n", product.ID, customer.ID, orders[0].ID)
		mustOK(t, newConsole(db, strings.NewReader(input), &output).run())
		assertStock(t, db, product.ID, 10)
		if !strings.Contains(output.String(), product.Name) || strings.Contains(output.String(), "ข้อผิดพลาด:") {
			t.Fatalf("console history/cancellation failed: %s", output.String())
		}
	})

	t.Run("SeedMinimumDataAndIdempotence", func(t *testing.T) {
		seedDB := newTestDatabase(t)
		seeded, err := seedDatabaseDB(seedDB)
		mustOK(t, err)
		if !seeded {
			t.Fatal("empty database was not seeded")
		}
		for _, check := range []struct {
			model any
			count int64
		}{
			{&Category{}, 4}, {&Product{}, 15}, {&Customer{}, 8}, {&Order{}, 12},
		} {
			var count int64
			mustOK(t, seedDB.Model(check.model).Count(&count).Error)
			if count != check.count {
				t.Fatalf("%T count = %d, want %d", check.model, count, check.count)
			}
		}
		products, err := getAllProductsDB(seedDB)
		mustOK(t, err)
		orders, err := getOrdersWithDetailsDB(seedDB)
		mustOK(t, err)
		// เทียบสต็อกจริงกับจำนวนตั้งต้น หักเฉพาะคำสั่งซื้อที่ไม่ถูกยกเลิก
		initial := []int{10, 15, 8, 4, 12, 10, 3, 30, 20, 18, 4, 50, 7, 14, 2}
		for i, product := range products {
			expected := initial[i]
			for _, order := range orders {
				if order.ProductID == product.ID && order.Status != StatusCancelled {
					expected -= order.Quantity
				}
			}
			if product.Quantity != expected {
				t.Fatalf("seed stock for %s = %d, want %d", product.Name, product.Quantity, expected)
			}
		}
		seeded, err = seedDatabaseDB(seedDB)
		mustOK(t, err)
		if seeded {
			t.Fatal("seed ran twice")
		}
		after, err := getAllProductsDB(seedDB)
		mustOK(t, err)
		if len(after) != len(products) {
			t.Fatal("seed duplicated products")
		}
		for i := range after {
			if after[i].Quantity != products[i].Quantity {
				t.Fatal("seed changed existing stock")
			}
		}
	})
}
