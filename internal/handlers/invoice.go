package handlers

import (
	"fmt"
	"html/template"
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
	err := db.DB.QueryRow(`SELECT i.id, i.invoice_number, i.project_id, p.name as project_name, i.client_id, c.company_name,
		c.contact_person, c.email, c.phone, i.issue_date, i.due_date, i.notes, i.status, i.subtotal, i.tax_rate, i.tax_amount, i.total, i.paid_amount
		FROM invoices i LEFT JOIN projects p ON i.project_id = p.id 
		LEFT JOIN clients c ON i.client_id = c.id WHERE i.id = ?`, id).
		Scan(&inv.ID, &inv.InvoiceNumber, &inv.ProjectID, &inv.ProjectName, &inv.ClientID, &inv.ClientName,
			&inv.ClientName, &inv.ClientName, &inv.ClientName, &inv.IssueDate, &inv.DueDate, &inv.Notes, &inv.Status,
			&inv.Subtotal, &inv.TaxRate, &inv.TaxAmount, &inv.Total, &inv.PaidAmount)
	if err != nil {
		http.Error(w, "发票不存在", http.StatusNotFound)
		return
	}

	var clientContact, clientEmail, clientPhone string
	db.DB.QueryRow("SELECT contact_person, email, phone FROM clients WHERE id = ?", inv.ClientID).
		Scan(&clientContact, &clientEmail, &clientPhone)

	inv.ClientName = inv.ClientName
	_ = clientContact

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
	db.DB.QueryRow(`SELECT i.id, i.invoice_number, i.project_id, p.name as project_name, i.client_id, c.company_name,
		i.issue_date, i.due_date, i.notes, i.status, i.subtotal, i.tax_rate, i.tax_amount, i.total, i.paid_amount
		FROM invoices i LEFT JOIN projects p ON i.project_id = p.id 
		LEFT JOIN clients c ON i.client_id = c.id WHERE i.id = ?`, id).
		Scan(&inv.ID, &inv.InvoiceNumber, &inv.ProjectID, &inv.ProjectName, &inv.ClientID, &inv.ClientName,
			&inv.IssueDate, &inv.DueDate, &inv.Notes, &inv.Status, &inv.Subtotal, &inv.TaxRate, &inv.TaxAmount, &inv.Total, &inv.PaidAmount)

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

	html := generateInvoiceHTML(inv)

	pdfDir := config.UploadDir + "/pdfs"
	os.MkdirAll(pdfDir, 0755)

	pdfPath := filepath.Join(pdfDir, fmt.Sprintf("invoice_%s.pdf", inv.InvoiceNumber))

	err := htmlToPDF(html, pdfPath)
	if err != nil {
		http.Error(w, "PDF generation failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "inline; filename=invoice_"+inv.InvoiceNumber+".pdf")
	http.ServeFile(w, r, pdfPath)
}

func generateInvoiceHTML(inv models.Invoice) string {
	itemsHTML := ""
	for _, item := range inv.Items {
		itemsHTML += fmt.Sprintf(`
			<tr>
				<td style="padding: 10px; border: 1px solid #ddd;">%s</td>
				<td style="padding: 10px; border: 1px solid #ddd; text-align: center;">%.2f</td>
				<td style="padding: 10px; border: 1px solid #ddd; text-align: right;">¥%.2f</td>
				<td style="padding: 10px; border: 1px solid #ddd; text-align: right;">¥%.2f</td>
			</tr>`, item.Description, item.Quantity, item.UnitPrice, item.Amount)
	}

	paymentsHTML := ""
	if len(inv.Payments) > 0 {
		paymentsHTML = `<h3 style="margin-top: 30px;">收款记录</h3><table style="width: 100%; border-collapse: collapse;">
			<tr style="background: #f5f5f5;"><th style="padding: 8px; border: 1px solid #ddd;">日期</th>
			<th style="padding: 8px; border: 1px solid #ddd;">方式</th>
			<th style="padding: 8px; border: 1px solid #ddd;">金额</th></tr>`
		for _, p := range inv.Payments {
			paymentsHTML += fmt.Sprintf(`<tr><td style="padding: 8px; border: 1px solid #ddd;">%s</td>
				<td style="padding: 8px; border: 1px solid #ddd;">%s</td>
				<td style="padding: 8px; border: 1px solid #ddd;">¥%.2f</td></tr>`, p.PaymentDate, p.Method, p.Amount)
		}
		paymentsHTML += "</table>"
	}

	html := fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="utf-8"></head><body style="font-family: 'Microsoft YaHei', Arial, sans-serif; padding: 40px;">
		<div style="max-width: 800px; margin: 0 auto;">
			<h1 style="text-align: center; border-bottom: 3px solid #3b82f6; padding-bottom: 10px;">发票</h1>
			<div style="margin-top: 20px;">
				<table style="width: 100%%; margin-bottom: 20px;">
					<tr><td style="width: 50%%;"><strong>发票号：</strong>%s</td><td><strong>开票日期：</strong>%s</td></tr>
					<tr><td><strong>客户：</strong>%s</td><td><strong>到期日期：</strong>%s</td></tr>
					<tr><td><strong>项目：</strong>%s</td><td><strong>状态：</strong>%s</td></tr>
				</table>
			</div>
			<table style="width: 100%%; border-collapse: collapse; margin-top: 20px;">
				<tr style="background: #3b82f6; color: white;">
					<th style="padding: 10px; border: 1px solid #ddd; text-align: left;">描述</th>
					<th style="padding: 10px; border: 1px solid #ddd; text-align: center;">数量</th>
					<th style="padding: 10px; border: 1px solid #ddd; text-align: right;">单价</th>
					<th style="padding: 10px; border: 1px solid #ddd; text-align: right;">金额</th>
				</tr>
				%s
			</table>
			<div style="margin-top: 20px; text-align: right;">
				<table style="margin-left: auto;">
					<tr><td style="padding: 8px;">小计：</td><td style="padding: 8px;">¥%.2f</td></tr>
					<tr><td style="padding: 8px;">税率 (%.1f%%)：</td><td style="padding: 8px;">¥%.2f</td></tr>
					<tr style="font-weight: bold; font-size: 18px;"><td style="padding: 8px;">总计：</td><td style="padding: 8px;">¥%.2f</td></tr>
					<tr><td style="padding: 8px;">已收：</td><td style="padding: 8px;">¥%.2f</td></tr>
					<tr style="color: #ef4444;"><td style="padding: 8px;">未收：</td><td style="padding: 8px;">¥%.2f</td></tr>
				</table>
			</div>
			%s
			%s
		</div></body></html>`,
		inv.InvoiceNumber, inv.IssueDate, inv.ClientName, inv.DueDate, inv.ProjectName, inv.Status,
		itemsHTML, inv.Subtotal, inv.TaxRate, inv.TaxAmount, inv.Total, inv.PaidAmount, inv.Total-inv.PaidAmount,
		paymentsHTML,
		func() string {
			if inv.Notes != "" {
				return fmt.Sprintf(`<div style="margin-top: 30px; padding: 15px; background: #f9fafb; border-radius: 5px;">
					<strong>备注：</strong>%s</div>`, inv.Notes)
			}
			return ""
		}())

	return template.HTMLEscapeString(html)
}

func htmlToPDF(html string, outputPath string) error {
	htmlPath := outputPath + ".html"
	err := os.WriteFile(htmlPath, []byte(html), 0644)
	if err != nil {
		return err
	}
	defer os.Remove(htmlPath)

	cmd := "chrome"
	args := []string{"--headless", "--disable-gpu", "--no-sandbox", "--print-to-pdf=" + outputPath, htmlPath}

	_ = cmd
	_ = args

	return fmt.Errorf("PDF generation requires Chrome/Chromium installed. Please install it and ensure 'chrome' is in PATH")
}
