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
	"github.com/signintech/gopdf"
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
		i.issue_date, i.due_date, i.notes, i.status, i.subtotal, i.tax_rate, i.tax_amount, i.total, i.paid_amount
		FROM invoices i LEFT JOIN projects p ON i.project_id = p.id 
		LEFT JOIN clients c ON i.client_id = c.id WHERE i.id = ?`, id).
		Scan(&inv.ID, &inv.InvoiceNumber, &inv.ProjectID, &inv.ProjectName, &inv.ClientID, &inv.ClientName,
			&inv.IssueDate, &inv.DueDate, &inv.Notes, &inv.Status,
			&inv.Subtotal, &inv.TaxRate, &inv.TaxAmount, &inv.Total, &inv.PaidAmount)
	if err != nil {
		http.Error(w, "发票不存在", http.StatusNotFound)
		return
	}

	var clientContact, clientEmail, clientPhone string
	db.DB.QueryRow("SELECT contact_person, email, phone FROM clients WHERE id = ?", inv.ClientID).
		Scan(&clientContact, &clientEmail, &clientPhone)

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

	var clientContact, clientEmail, clientPhone string
	db.DB.QueryRow("SELECT contact_person, email, phone FROM clients WHERE id = ?", inv.ClientID).
		Scan(&clientContact, &clientEmail, &clientPhone)

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

func generateInvoicePDF(inv models.Invoice, clientContact, clientEmail, clientPhone, outputPath string) error {
	pdf := gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4, Unit: gopdf.UnitMM})

	fontPath := "C:\\Windows\\Fonts\\simhei.ttf"
	if err := pdf.AddTTFFont("simhei", fontPath); err != nil {
		return fmt.Errorf("failed to load font: %w", err)
	}

	marginLeft := 15.0
	marginRight := 15.0
	pageWidth := 210.0
	contentWidth := pageWidth - marginLeft - marginRight

	pdf.AddPage()

	pdf.SetFont("simhei", "", 24)
	pdf.SetTextColor(59, 130, 246)
	title := "发  票"
	titleWidth := 40.0
	pdf.SetX((pageWidth - titleWidth) / 2)
	pdf.Cell(nil, title)
	pdf.Br(12)

	pdf.SetStrokeColor(59, 130, 246)
	pdf.SetLineWidth(0.8)
	pdf.Line(marginLeft, pdf.GetY(), pageWidth-marginRight, pdf.GetY())
	pdf.Br(10)

	pdf.SetFont("simhei", "", 10)
	pdf.SetTextColor(0, 0, 0)

	infoData := [][]string{
		{"发票号：", inv.InvoiceNumber, "开票日期：", inv.IssueDate},
		{"客户：", inv.ClientName, "到期日期：", inv.DueDate},
		{"项目：", inv.ProjectName, "状态：", inv.Status},
	}

	colWidths := []float64{20, 65, 20, 65}

	for _, row := range infoData {
		pdf.SetX(marginLeft)
		for i, cell := range row {
			cellWidth := colWidths[i]
			if i == 1 || i == 3 {
				pdf.SetTextColor(80, 80, 80)
			} else {
				pdf.SetTextColor(0, 0, 0)
			}
			pdf.CellWithOption(&gopdf.Rect{W: cellWidth, H: 8}, cell, gopdf.CellOption{Align: gopdf.Left})
		}
		pdf.Br(10)
	}

	pdf.SetTextColor(0, 0, 0)
	if clientContact != "" || clientEmail != "" || clientPhone != "" {
		pdf.SetFont("simhei", "", 9)
		pdf.SetTextColor(100, 100, 100)
		contactInfo := ""
		if clientContact != "" {
			contactInfo += "联系人: " + clientContact + "  "
		}
		if clientEmail != "" {
			contactInfo += "邮箱: " + clientEmail + "  "
		}
		if clientPhone != "" {
			contactInfo += "电话: " + clientPhone
		}
		pdf.SetX(marginLeft)
		pdf.Cell(nil, contactInfo)
		pdf.Br(10)
	}

	pdf.SetFont("simhei", "", 10)
	headerY := pdf.GetY()

	headers := []string{"描述", "数量", "单价", "金额"}
	headerWidths := []float64{contentWidth * 0.5, contentWidth * 0.15, contentWidth * 0.175, contentWidth * 0.175}

	pdf.SetFillColor(59, 130, 246)
	pdf.SetTextColor(255, 255, 255)
	currentX := marginLeft
	for i, header := range headers {
		pdf.RectFromUpperLeftWithStyle(currentX, headerY, headerWidths[i], 10, "F")
		pdf.SetX(currentX + 2)
		pdf.SetY(headerY + 2)
		if i == 0 {
			pdf.CellWithOption(&gopdf.Rect{W: headerWidths[i] - 4, H: 6}, header, gopdf.CellOption{Align: gopdf.Left})
		} else if i == 1 {
			pdf.CellWithOption(&gopdf.Rect{W: headerWidths[i] - 4, H: 6}, header, gopdf.CellOption{Align: gopdf.Center})
		} else {
			pdf.CellWithOption(&gopdf.Rect{W: headerWidths[i] - 4, H: 6}, header, gopdf.CellOption{Align: gopdf.Right})
		}
		currentX += headerWidths[i]
	}
	pdf.SetY(headerY + 10)

	pdf.SetStrokeColor(200, 200, 200)
	pdf.SetLineWidth(0.3)

	rowHeight := 8.0
	for idx, item := range inv.Items {
		rowY := pdf.GetY()

		if pdf.GetY() > 270 {
			pdf.AddPage()
			pdf.SetFont("simhei", "", 10)
			rowY = pdf.GetY()
		}

		if idx%2 == 1 {
			pdf.SetFillColor(249, 250, 251)
			pdf.RectFromUpperLeftWithStyle(marginLeft, rowY, contentWidth, rowHeight, "F")
		}

		pdf.SetTextColor(0, 0, 0)
		pdf.SetX(marginLeft + 2)
		pdf.CellWithOption(&gopdf.Rect{W: headerWidths[0] - 4, H: rowHeight}, item.Description, gopdf.CellOption{Align: gopdf.Left})

		pdf.SetX(marginLeft + headerWidths[0])
		pdf.CellWithOption(&gopdf.Rect{W: headerWidths[1] - 4, H: rowHeight}, fmt.Sprintf("%.2f", item.Quantity), gopdf.CellOption{Align: gopdf.Center})

		pdf.SetX(marginLeft + headerWidths[0] + headerWidths[1])
		pdf.CellWithOption(&gopdf.Rect{W: headerWidths[2] - 4, H: rowHeight}, fmt.Sprintf("%.2f", item.UnitPrice), gopdf.CellOption{Align: gopdf.Right})

		pdf.SetX(marginLeft + headerWidths[0] + headerWidths[1] + headerWidths[2])
		pdf.CellWithOption(&gopdf.Rect{W: headerWidths[3] - 4, H: rowHeight}, fmt.Sprintf("%.2f", item.Amount), gopdf.CellOption{Align: gopdf.Right})

		pdf.Line(marginLeft, rowY+rowHeight, marginLeft+contentWidth, rowY+rowHeight)
		pdf.SetY(rowY + rowHeight)
	}

	pdf.Br(8)

	pdf.SetFont("simhei", "", 10)
	pdf.SetTextColor(0, 0, 0)

	summaryData := [][]string{
		{"小计：", fmt.Sprintf("¥%.2f", inv.Subtotal)},
		{fmt.Sprintf("税额 (%.1f%%)：", inv.TaxRate), fmt.Sprintf("¥%.2f", inv.TaxAmount)},
		{"总计：", fmt.Sprintf("¥%.2f", inv.Total)},
		{"已收：", fmt.Sprintf("¥%.2f", inv.PaidAmount)},
		{"未收：", fmt.Sprintf("¥%.2f", inv.Total-inv.PaidAmount)},
	}

	labelWidth := 40.0
	valueWidth := 40.0
	rightStartX := pageWidth - marginRight - labelWidth - valueWidth

	for i, row := range summaryData {
		if i == 2 {
			pdf.SetFont("simhei", "", 12)
		} else {
			pdf.SetFont("simhei", "", 10)
		}
		if i == 4 {
			pdf.SetTextColor(239, 68, 68)
		} else {
			pdf.SetTextColor(0, 0, 0)
		}
		pdf.SetX(rightStartX)
		pdf.CellWithOption(&gopdf.Rect{W: labelWidth, H: 8}, row[0], gopdf.CellOption{Align: gopdf.Right})
		pdf.SetX(rightStartX + labelWidth)
		pdf.CellWithOption(&gopdf.Rect{W: valueWidth, H: 8}, row[1], gopdf.CellOption{Align: gopdf.Right})
		pdf.Br(9)
	}

	if inv.Notes != "" {
		pdf.Br(5)
		pdf.SetFont("simhei", "", 9)
		pdf.SetTextColor(100, 100, 100)
		pdf.SetX(marginLeft)
		notesY := pdf.GetY()
		pdf.SetFillColor(249, 250, 251)
		pdf.RectFromUpperLeftWithStyle(marginLeft, notesY, contentWidth, 20, "F")
		pdf.SetY(notesY + 2)
		pdf.Cell(nil, "备注：")
		pdf.Br(6)
		pdf.SetX(marginLeft + 2)
		pdf.Cell(nil, inv.Notes)
		pdf.SetY(notesY + 22)
	}

	if len(inv.Payments) > 0 {
		pdf.Br(8)
		pdf.SetFont("simhei", "", 11)
		pdf.SetTextColor(59, 130, 246)
		pdf.SetX(marginLeft)
		pdf.Cell(nil, "收款记录")
		pdf.Br(8)

		pdf.SetStrokeColor(200, 200, 200)
		pdf.SetLineWidth(0.3)

		payHeaderY := pdf.GetY()
		payHeaders := []string{"日期", "方式", "金额"}
		payWidths := []float64{contentWidth * 0.4, contentWidth * 0.3, contentWidth * 0.3}

		pdf.SetFillColor(245, 245, 245)
		pdf.SetFont("simhei", "", 9)
		pdf.SetTextColor(0, 0, 0)
		curX := marginLeft
		for i, h := range payHeaders {
			pdf.RectFromUpperLeftWithStyle(curX, payHeaderY, payWidths[i], 7, "F")
			pdf.SetX(curX + 2)
			pdf.SetY(payHeaderY + 1)
			if i == 0 {
				pdf.CellWithOption(&gopdf.Rect{W: payWidths[i] - 4, H: 5}, h, gopdf.CellOption{Align: gopdf.Left})
			} else if i == 1 {
				pdf.CellWithOption(&gopdf.Rect{W: payWidths[i] - 4, H: 5}, h, gopdf.CellOption{Align: gopdf.Center})
			} else {
				pdf.CellWithOption(&gopdf.Rect{W: payWidths[i] - 4, H: 5}, h, gopdf.CellOption{Align: gopdf.Right})
			}
			curX += payWidths[i]
		}
		pdf.SetY(payHeaderY + 7)

		pdf.SetFont("simhei", "", 9)
		for _, p := range inv.Payments {
			pRowY := pdf.GetY()
			pdf.SetX(marginLeft + 2)
			pdf.CellWithOption(&gopdf.Rect{W: payWidths[0] - 4, H: 7}, p.PaymentDate, gopdf.CellOption{Align: gopdf.Left})
			pdf.SetX(marginLeft + payWidths[0])
			pdf.CellWithOption(&gopdf.Rect{W: payWidths[1] - 4, H: 7}, p.Method, gopdf.CellOption{Align: gopdf.Center})
			pdf.SetX(marginLeft + payWidths[0] + payWidths[1])
			pdf.CellWithOption(&gopdf.Rect{W: payWidths[2] - 4, H: 7}, fmt.Sprintf("¥%.2f", p.Amount), gopdf.CellOption{Align: gopdf.Right})
			pdf.Line(marginLeft, pRowY+7, marginLeft+contentWidth, pRowY+7)
			pdf.SetY(pRowY + 7)
		}
	}

	return pdf.WritePdf(outputPath)
}
