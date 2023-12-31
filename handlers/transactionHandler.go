package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"

	_ "github.com/go-sql-driver/mysql"
)

func AddTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	err := r.ParseForm()
	if err != nil {
		log.Println("Failed to parse form data:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	bookTitle := r.Form.Get("bookTitle")
	quantityStr := r.Form.Get("quantity")
	staffName := r.Form.Get("staffName")

	quantity, err := strconv.Atoi(quantityStr)
	if err != nil {
		log.Println("Invalid quantity:", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	db, err := sql.Open("mysql", "root:@tcp(127.0.0.1:3306)/books_store")
	if err != nil {
		log.Println("Failed to connect to the database:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Retrieve staff details from the staff table
	staffQuery := "SELECT staff_id FROM staff WHERE name = ?"
	staffStmt, err := db.Prepare(staffQuery)
	if err != nil {
		log.Println("Failed to prepare SQL statement for retrieving staff details:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer staffStmt.Close()
	var staffID int

	err = staffStmt.QueryRow(staffName).Scan(&staffID)
	if err != nil {
		log.Println("Failed to execute SQL statement for retrieving book details:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Retrieve book details from the books table
	bookQuery := "SELECT book_id, price, stock_qty FROM books WHERE title = ?"
	bookStmt, err := db.Prepare(bookQuery)
	if err != nil {
		log.Println("Failed to prepare SQL statement for retrieving book details:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer bookStmt.Close()

	var bookID int
	var itemPrice float64
	var stockQty int
	err = bookStmt.QueryRow(bookTitle).Scan(&bookID, &itemPrice, &stockQty)
	if err != nil {
		log.Println("Failed to execute SQL statement for retrieving book details:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if quantity > stockQty {
		log.Println("Insufficient stock quantity")
		http.Error(w, "Insufficient stock quantity", http.StatusBadRequest)
		return
	}

	// Update stock quantity in books table
	updateQtyStmt, err := db.Prepare("UPDATE books SET stock_qty = stock_qty - ? WHERE book_id = ?")
	if err != nil {
		log.Println("Failed to prepare SQL statement for updating stock quantity:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer updateQtyStmt.Close()

	_, err = updateQtyStmt.Exec(quantity, bookID)
	if err != nil {
		log.Println("Failed to execute SQL statement for updating stock quantity:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Insert into cart table
	cartInsertStmt, err := db.Prepare("INSERT INTO cart (cart_id, book_id, qty, item_price) VALUES (?, ?, ?, ?)")
	if err != nil {
		log.Println("Failed to prepare SQL statement for inserting into cart table:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer cartInsertStmt.Close()

	// Generate a unique cart ID
	cartID := generateUniqueID(bookTitle + staffName)

	_, err = cartInsertStmt.Exec(cartID, bookID, quantity, itemPrice)
	if err != nil {
		log.Println("Failed to execute SQL statement for inserting into cart table:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Calculate the total price
	totalPrice := float64(quantity) * itemPrice

	// Insert into transaction table
	transInsertStmt, err := db.Prepare("INSERT INTO transactions (trans_id, cart_id, trans_date, total_price, staff_id) VALUES (?, ?, now(), ?, ?)")
	if err != nil {
		log.Println("Failed to prepare SQL statement for inserting into transaction table:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer transInsertStmt.Close()

	// Generate a unique transaction ID
	transID := generateUniqueID(staffName + bookTitle + "ABC")

	_, err = transInsertStmt.Exec(transID, cartID, totalPrice, staffID)
	if err != nil {
		log.Println("Failed to execute SQL statement for inserting into transactions table:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintln(w, "Transaction added successfully!")
}

func DeleteTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the form data
	err := r.ParseForm()
	if err != nil {
		log.Println("Failed to parse form data:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Get the cart ID from the form data
	cartID := r.Form.Get("cartID")

	db, err := sql.Open("mysql", "root:@tcp(127.0.0.1:3306)/books_store")
	if err != nil {
		log.Println("Failed to connect to the database:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	// Start a transaction to ensure atomicity
	tx, err := db.Begin()
	if err != nil {
		log.Println("Failed to start transaction:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Delete the cart from the database
	stmtDeleteCart, err := tx.Prepare("DELETE FROM cart WHERE cart_id = ?")
	if err != nil {
		log.Println("Failed to prepare SQL statement for deleting cart:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		_ = tx.Rollback()
		return
	}
	defer stmtDeleteCart.Close()

	_, err = stmtDeleteCart.Exec(cartID)
	if err != nil {
		log.Println("Failed to execute SQL statement for deleting cart:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		_ = tx.Rollback()
		return
	}

	// Delete the associated transaction from the database
	stmtDeleteTransaction, err := tx.Prepare("DELETE FROM transactions WHERE cart_id = ?")
	if err != nil {
		log.Println("Failed to prepare SQL statement for deleting transaction:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		_ = tx.Rollback()
		return
	}
	defer stmtDeleteTransaction.Close()

	_, err = stmtDeleteTransaction.Exec(cartID)
	if err != nil {
		log.Println("Failed to execute SQL statement for deleting transaction:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		_ = tx.Rollback()
		return
	}

	// Commit the transaction
	err = tx.Commit()
	if err != nil {
		log.Println("Failed to commit transaction:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	fmt.Fprintln(w, "Transaction deleted successfully!")
}
