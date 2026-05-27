package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/freelance/workbench/internal/config"
	"github.com/freelance/workbench/internal/db"
	"github.com/freelance/workbench/internal/middleware"
	"github.com/freelance/workbench/internal/models"
	"github.com/freelance/workbench/internal/utils"
	"github.com/jung-kurt/gofpdf"
)

func ListInvoices(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	statusFilter := r.URL.Query().Get("status")
	clientFilter := r.URL.Query().Get("client_id")

	query := `SELECT i.id, i.invoice_number, i.project_id, p.name as project_name, i.client_id, c.company_name, 
		i.issue_date, i.due_date, i.status, i.subtotal, i.tax_rate, i.tax_amount, i.total, i.paid_amount, i.created_at
		FROM invoices i LEFT JOIN projects p ON i.project_id = p.id 
		LEFT JOIN clients c ON i.client_id = c.id WHERE 1=1`
	var args []interface{}

	if statusFilter != "" {
		query += " AND i.status = ?"
		args = append(args, statusFilter)
	}
	if clientFilter != "" {
		query += " AND i.client_id = ?"
		args = append(args, parseInt64(clientFilter))
	}

	query += " ORDER BY i.created_at DESC"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var invoices []models.Invoice
	for rows.Next() {
		var inv models.Invoice
		rows.Scan(&inv.ID, &inv.InvoiceNumber, &inv.ProjectID, &inv.ProjectName, &inv.ClientID, &inv.ClientName,
			&inv.IssueDate, &inv.DueDate, &inv.Status, &inv.Subtotal, &inv.TaxRate, &inv.TaxAmount, &inv.Total, &inv.PaidAmount, &inv.CreatedAt)

		if inv.DueDate != "" && inv.Status == "已发送" {
			due, _ := time.Parse("2006-01-02", inv.DueDate)
			if due.Before(time.Now()) {
				inv.Status = "已逾期"
				db.DB.Exec("UPDATE invoices SET status = '已逾期' WHERE id = ?", inv.ID)
			}
		}

		invoices = append(invoices, inv)
	}

	var clients []models.Client
	clientRows, _ := db.DB.Query("SELECT id, company_name FROM clients ORDER BY company_name")
	for clientRows.Next() {
		var c models.Client
		clientRows.Scan(&c.ID, &c.CompanyName)
		clients = append(clients, c)
	}
	clientRows.Close()

	renderTemplate(w, "invoices.html", TemplateData{
		Title:  "发票管理",
		User:   user,
		Data:   map[string]interface{}{"invoices": invoices, "clients": clients, "statusFilter": statusFilter, "clientFilter": clientFilter},
		Active: "invoices",
	})
}

func NewInvoiceForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)

	var clients []models.Client
	clientRows, _ := db.DB.Query("SELECT id, company_name FROM clients ORDER BY company_name")
	for clientRows.Next() {
		var c models.Client
		clientRows.Scan(&c.ID, &c.CompanyName)
		clients = append(clients, c)
	}
	clientRows.Close()

	var projects []models.Project
	projRows, _ := db.DB.Query("SELECT id, name, client_id FROM projects ORDER BY name")
	for projRows.Next() {
		var p models.Project
		projRows.Scan(&p.ID, &p.Name, &p.ClientID)
		projects = append(projects, p)
	}
	projRows.Close()

	renderTemplate(w, "invoice_form.html", TemplateData{
		Title:  "新建发票",
		User:   user,
		Data:   map[string]interface{}{"clients": clients, "projects": projects, "invoice": nil, "taxRate": user.TaxRate},
		Active: "invoices",
	})
}

func CreateInvoice(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/invoices/new", http.StatusSeeOther)
		return
	}

	clientID := parseInt64(r.FormValue("client_id"))
	projectID := parseInt64(r.FormValue("project_id"))
	issueDate := r.FormValue("issue_date")
	dueDate := r.FormValue("due_date")
	notes := r.FormValue("notes")
	taxRate := parseFloat(r.FormValue("tax_rate"))

	descriptions := r.Form["item_description[]"]
	quantities := r.Form["item_quantity[]"]
	unitPrices := r.Form["item_unit_price[]"]

	invoiceNumber := utils.GenerateInvoiceNumber()

	var subtotal float64
	for i := range descriptions {
		qty := parseFloat(quantities[i])
		price := parseFloat(unitPrices[i])
		subtotal += qty * price
	}

	taxAmount := subtotal * taxRate / 100
	total := subtotal + taxAmount

	result, err := db.DB.Exec(`INSERT INTO invoices (invoice_number, project_id, client_id, issue_date, due_date, notes, tax_rate, subtotal, tax_amount, total) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, invoiceNumber, projectID, clientID, issueDate, dueDate, notes, taxRate, subtotal, taxAmount, total)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	invoiceID, _ := result.LastInsertId()

	for i := range descriptions {
		qty := parseFloat(quantities[i])
		price := parseFloat(unitPrices[i])
		amount := qty * price
		db.DB.Exec("INSERT INTO invoice_items (invoice_id, description, quantity, unit_price, amount) VALUES (?, ?, ?, ?, ?)",
			invoiceID, descriptions[i], qty, price, amount)
	}

	http.Redirect(w, r, "/invoices/detail?id="+strconv.FormatInt(invoiceID, 10), http.StatusSeeOther)
}

func InvoiceDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	id := parseInt64(r.URL.Query().Get("id"))

	var inv models.Invoice
	var clientContact, clientEmail, clientPhone string
	err := db.DB.QueryRow(`SELECT i.id, i.invoice_number, i.project_id, p.name as project_name, i.client_id, c.company_name,
		c.contact_person, c.email, c.phone, i.issue_date, i.due_date, i.notes, i.status, i.subtotal, i.tax_rate, i.tax_amount, i.total, i.paid_amount
		FROM invoices i LEFT JOIN projects p ON i.project_id = p.id 
		LEFT JOIN clients c ON i.client_id = c.id WHERE i.id = ?`, id).
		Scan(&inv.ID, &inv.InvoiceNumber, &inv.ProjectID, &inv.ProjectName, &inv.ClientID, &inv.ClientName,
			&clientContact, &clientEmail, &clientPhone, &inv.IssueDate, &inv.DueDate, &inv.Notes, &inv.Status,
			&inv.Subtotal, &inv.TaxRate, &inv.TaxAmount, &inv.Total, &inv.PaidAmount)
	if err != nil {
		http.Error(w, "发票不存在", http.StatusNotFound)
		return
	}

	itemRows, _ := db.DB.Query("SELECT id, description, quantity, unit_price, amount FROM invoice_items WHERE invoice_id = ?", id)
	for itemRows.Next() {
		var item models.InvoiceItem
		itemRows.Scan(&item.ID, &item.Description, &item.Quantity, &item.UnitPrice, &item.Amount)
		inv.Items = append(inv.Items, item)
	}
	itemRows.Close()

	payRows, _ := db.DB.Query("SELECT id, amount, payment_date, method, notes FROM payments WHERE invoice_id = ? ORDER BY payment_date", id)
	for payRows.Next() {
		var p models.Payment
		payRows.Scan(&p.ID, &p.Amount, &p.PaymentDate, &p.Method, &p.Notes)
		inv.Payments = append(inv.Payments, p)
	}
	payRows.Close()

	renderTemplate(w, "invoice_detail.html", TemplateData{
		Title:  "发票详情 - " + inv.InvoiceNumber,
		User:   user,
		Data:   map[string]interface{}{"invoice": inv, "clientContact": clientContact, "clientEmail": clientEmail, "clientPhone": clientPhone},
		Active: "invoices",
	})
}

func UpdateInvoiceStatus(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	status := r.URL.Query().Get("status")

	db.DB.Exec("UPDATE invoices SET status = ? WHERE id = ?", status, id)
	http.Redirect(w, r, "/invoices/detail?id="+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func DeleteInvoice(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	db.DB.Exec("DELETE FROM invoices WHERE id = ?", id)
	http.Redirect(w, r, "/invoices", http.StatusSeeOther)
}

func AddPayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/invoices", http.StatusSeeOther)
		return
	}

	invoiceID := parseInt64(r.FormValue("invoice_id"))
	amount := parseFloat(r.FormValue("amount"))
	paymentDate := r.FormValue("payment_date")
	method := r.FormValue("method")
	notes := r.FormValue("notes")

	db.DB.Exec("INSERT INTO payments (invoice_id, amount, payment_date, method, notes) VALUES (?, ?, ?, ?, ?)",
		invoiceID, amount, paymentDate, method, notes)

	var totalPaid float64
	db.DB.QueryRow("SELECT COALESCE(SUM(amount), 0) FROM payments WHERE invoice_id = ?", invoiceID).Scan(&totalPaid)
	db.DB.Exec("UPDATE invoices SET paid_amount = ? WHERE id = ?", totalPaid, invoiceID)

	var invoiceTotal float64
	db.DB.QueryRow("SELECT total FROM invoices WHERE id = ?", invoiceID).Scan(&invoiceTotal)
	if totalPaid >= invoiceTotal {
		db.DB.Exec("UPDATE invoices SET status = '已支付' WHERE id = ?", invoiceID)
	}

	http.Redirect(w, r, "/invoices/detail?id="+r.FormValue("invoice_id"), http.StatusSeeOther)
}

func DeletePayment(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	invoiceID := r.URL.Query().Get("invoice_id")

	db.DB.Exec("DELETE FROM payments WHERE id = ?", id)

	var totalPaid float64
	db.DB.QueryRow("SELECT COALESCE(SUM(amount), 0) FROM payments WHERE invoice_id = ?", parseInt64(invoiceID)).Scan(&totalPaid)
	db.DB.Exec("UPDATE invoices SET paid_amount = ? WHERE id = ?", totalPaid, parseInt64(invoiceID))

	var invoiceTotal float64
	db.DB.QueryRow("SELECT total FROM invoices WHERE id = ?", parseInt64(invoiceID)).Scan(&invoiceTotal)
	if totalPaid < invoiceTotal {
		db.DB.Exec("UPDATE invoices SET status = '已发送' WHERE id = ? AND status = '已支付'", parseInt64(invoiceID))
	}

	http.Redirect(w, r, "/invoices/detail?id="+invoiceID, http.StatusSeeOther)
}

func GenerateInvoicePDF(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))

	var inv models.Invoice
	var clientContact, clientEmail, clientPhone string
	db.DB.QueryRow(`SELECT i.id, i.invoice_number, i.project_id, p.name as project_name, i.client_id, c.company_name,
		c.contact_person, c.email, c.phone, i.issue_date, i.due_date, i.notes, i.status, i.subtotal, i.tax_rate, i.tax_amount, i.total, i.paid_amount
		FROM invoices i LEFT JOIN projects p ON i.project_id = p.id 
		LEFT JOIN clients c ON i.client_id = c.id WHERE i.id = ?`, id).
		Scan(&inv.ID, &inv.InvoiceNumber, &inv.ProjectID, &inv.ProjectName, &inv.ClientID, &inv.ClientName,
			&clientContact, &clientEmail, &clientPhone, &inv.IssueDate, &inv.DueDate, &inv.Notes, &inv.Status,
			&inv.Subtotal, &inv.TaxRate, &inv.TaxAmount, &inv.Total, &inv.PaidAmount)

	itemRows, _ := db.DB.Query("SELECT id, description, quantity, unit_price, amount FROM invoice_items WHERE invoice_id = ?", id)
	for itemRows.Next() {
		var item models.InvoiceItem
		itemRows.Scan(&item.ID, &item.Description, &item.Quantity, &item.UnitPrice, &item.Amount)
		inv.Items = append(inv.Items, item)
	}
	itemRows.Close()

	payRows, _ := db.DB.Query("SELECT id, amount, payment_date, method, notes FROM payments WHERE invoice_id = ? ORDER BY payment_date", id)
	for payRows.Next() {
		var p models.Payment
		payRows.Scan(&p.ID, &p.Amount, &p.PaymentDate, &p.Method, &p.Notes)
		inv.Payments = append(inv.Payments, p)
	}
	payRows.Close()

	pdfDir := config.UploadDir + "/pdfs"
	os.MkdirAll(pdfDir, 0755)

	pdfPath := filepath.Join(pdfDir, fmt.Sprintf("invoice_%s.pdf", inv.InvoiceNumber))

	err := generateInvoicePDF(inv, clientContact, clientEmail, clientPhone, pdfPath)
	if err != nil {
		http.Error(w, "PDF generation failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=invoice_"+inv.InvoiceNumber+".pdf")
	http.ServeFile(w, r, pdfPath)
}

func generateInvoicePDF(inv models.Invoice, clientContact, clientEmail, clientPhone string, outputPath string) error {
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 24)
	pdf.CellFormat(0, 15, "INVOICE", "", 1, "C", false, 0, "")
	pdf.Ln(5)

	pdf.SetDrawColor(59, 130, 246)
	pdf.SetLineWidth(1)
	pdf.Line(10, pdf.GetY(), 200, pdf.GetY())
	pdf.Ln(10)

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(40, 8, "Invoice Number:", "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(60, 8, inv.InvoiceNumber, "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(35, 8, "Issue Date:", "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(55, 8, inv.IssueDate, "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(40, 8, "Client:", "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(60, 8, inv.ClientName, "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(35, 8, "Due Date:", "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(55, 8, inv.DueDate, "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(40, 8, "Project:", "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(60, 8, inv.ProjectName, "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "B", 12)
	pdf.CellFormat(35, 8, "Status:", "", 0, "L", false, 0, "")
	pdf.SetFont("Arial", "", 12)
	pdf.CellFormat(55, 8, inv.Status, "", 1, "L", false, 0, "")

	if clientContact != "" || clientEmail != "" || clientPhone != "" {
		pdf.Ln(5)
		pdf.SetFont("Arial", "B", 11)
		pdf.CellFormat(40, 7, "Contact Info:", "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		if clientContact != "" {
			pdf.CellFormat(40, 6, "Contact:", "", 0, "L", false, 0, "")
			pdf.CellFormat(60, 6, clientContact, "", 1, "L", false, 0, "")
		}
		if clientEmail != "" {
			pdf.CellFormat(40, 6, "Email:", "", 0, "L", false, 0, "")
			pdf.CellFormat(60, 6, clientEmail, "", 1, "L", false, 0, "")
		}
		if clientPhone != "" {
			pdf.CellFormat(40, 6, "Phone:", "", 0, "L", false, 0, "")
			pdf.CellFormat(60, 6, clientPhone, "", 1, "L", false, 0, "")
		}
	}

	pdf.Ln(10)

	pdf.SetFont("Arial", "B", 11)
	pdf.SetFillColor(59, 130, 246)
	pdf.SetTextColor(255, 255, 255)
	pdf.CellFormat(80, 10, "Description", "1", 0, "C", true, 0, "")
	pdf.CellFormat(25, 10, "Quantity", "1", 0, "C", true, 0, "")
	pdf.CellFormat(35, 10, "Unit Price", "1", 0, "C", true, 0, "")
	pdf.CellFormat(50, 10, "Amount", "1", 1, "C", true, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(0, 0, 0)
	for _, item := range inv.Items {
		pdf.CellFormat(80, 9, item.Description, "1", 0, "L", false, 0, "")
		pdf.CellFormat(25, 9, fmt.Sprintf("%.2f", item.Quantity), "1", 0, "C", false, 0, "")
		pdf.CellFormat(35, 9, fmt.Sprintf("¥%.2f", item.UnitPrice), "1", 0, "R", false, 0, "")
		pdf.CellFormat(50, 9, fmt.Sprintf("¥%.2f", item.Amount), "1", 1, "R", false, 0, "")
	}

	pdf.Ln(5)
	pdf.SetFont("Arial", "", 11)
	pdf.CellFormat(140, 8, "Subtotal:", "", 0, "R", false, 0, "")
	pdf.CellFormat(50, 8, fmt.Sprintf("¥%.2f", inv.Subtotal), "", 1, "R", false, 0, "")

	pdf.CellFormat(140, 8, fmt.Sprintf("Tax (%.1f%%):", inv.TaxRate), "", 0, "R", false, 0, "")
	pdf.CellFormat(50, 8, fmt.Sprintf("¥%.2f", inv.TaxAmount), "", 1, "R", false, 0, "")

	pdf.SetFont("Arial", "B", 14)
	pdf.CellFormat(140, 10, "Total:", "", 0, "R", false, 0, "")
	pdf.CellFormat(50, 10, fmt.Sprintf("¥%.2f", inv.Total), "", 1, "R", false, 0, "")

	pdf.SetFont("Arial", "", 11)
	pdf.CellFormat(140, 8, "Paid:", "", 0, "R", false, 0, "")
	pdf.CellFormat(50, 8, fmt.Sprintf("¥%.2f", inv.PaidAmount), "", 1, "R", false, 0, "")

	pdf.SetTextColor(239, 68, 68)
	pdf.CellFormat(140, 8, "Unpaid:", "", 0, "R", false, 0, "")
	pdf.CellFormat(50, 8, fmt.Sprintf("¥%.2f", inv.Total-inv.PaidAmount), "", 1, "R", false, 0, "")
	pdf.SetTextColor(0, 0, 0)

	if len(inv.Payments) > 0 {
		pdf.Ln(10)
		pdf.SetFont("Arial", "B", 12)
		pdf.CellFormat(0, 10, "Payment History", "", 1, "L", false, 0, "")

		pdf.SetFont("Arial", "B", 10)
		pdf.SetFillColor(245, 245, 245)
		pdf.CellFormat(60, 8, "Date", "1", 0, "C", true, 0, "")
		pdf.CellFormat(60, 8, "Method", "1", 0, "C", true, 0, "")
		pdf.CellFormat(70, 8, "Amount", "1", 1, "C", true, 0, "")

		pdf.SetFont("Arial", "", 10)
		for _, p := range inv.Payments {
			pdf.CellFormat(60, 8, p.PaymentDate, "1", 0, "C", false, 0, "")
			pdf.CellFormat(60, 8, p.Method, "1", 0, "C", false, 0, "")
			pdf.CellFormat(70, 8, fmt.Sprintf("¥%.2f", p.Amount), "1", 1, "R", false, 0, "")
		}
	}

	if inv.Notes != "" {
		pdf.Ln(10)
		pdf.SetFont("Arial", "B", 11)
		pdf.CellFormat(0, 8, "Notes:", "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 10)
		pdf.SetFillColor(249, 250, 251)
		pdf.MultiCell(0, 7, inv.Notes, "1", "L", true)
	}

	return pdf.OutputFileAndClose(outputPath)
}
