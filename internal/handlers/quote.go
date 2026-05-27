package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/freelance/workbench/internal/db"
	"github.com/freelance/workbench/internal/middleware"
	"github.com/freelance/workbench/internal/models"
	"github.com/freelance/workbench/internal/utils"
)

func ListQuotes(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	statusFilter := r.URL.Query().Get("status")

	query := `SELECT q.id, q.quote_number, q.project_id, p.name as project_name, q.client_id, c.company_name,
		q.issue_date, q.valid_until, q.status, q.total, q.invoice_id, q.created_at
		FROM quotes q LEFT JOIN projects p ON q.project_id = p.id 
		LEFT JOIN clients c ON q.client_id = c.id WHERE 1=1`
	var args []interface{}

	if statusFilter != "" {
		query += " AND q.status = ?"
		args = append(args, statusFilter)
	}

	query += " ORDER BY q.created_at DESC"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var quotes []models.Quote
	for rows.Next() {
		var q models.Quote
		rows.Scan(&q.ID, &q.QuoteNumber, &q.ProjectID, &q.ProjectName, &q.ClientID, &q.ClientName,
			&q.IssueDate, &q.ValidUntil, &q.Status, &q.Total, &q.InvoiceID, &q.CreatedAt)
		quotes = append(quotes, q)
	}

	var templates []models.QuoteTemplate
	tplRows, _ := db.DB.Query("SELECT id, name, description, items, tax_rate, notes FROM quote_templates ORDER BY name")
	for tplRows.Next() {
		var t models.QuoteTemplate
		tplRows.Scan(&t.ID, &t.Name, &t.Description, &t.Items, &t.TaxRate, &t.Notes)
		templates = append(templates, t)
	}
	tplRows.Close()

	renderTemplate(w, "quotes.html", TemplateData{
		Title:  "报价单",
		User:   user,
		Data:   map[string]interface{}{"quotes": quotes, "templates": templates, "statusFilter": statusFilter},
		Active: "quotes",
	})
}

func NewQuoteForm(w http.ResponseWriter, r *http.Request) {
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

	var templates []models.QuoteTemplate
	tplRows, _ := db.DB.Query("SELECT id, name, description, items, tax_rate, notes FROM quote_templates ORDER BY name")
	for tplRows.Next() {
		var t models.QuoteTemplate
		tplRows.Scan(&t.ID, &t.Name, &t.Description, &t.Items, &t.TaxRate, &t.Notes)
		templates = append(templates, t)
	}
	tplRows.Close()

	renderTemplate(w, "quote_form.html", TemplateData{
		Title:  "新建报价单",
		User:   user,
		Data:   map[string]interface{}{"clients": clients, "projects": projects, "templates": templates, "quote": nil, "taxRate": user.TaxRate},
		Active: "quotes",
	})
}

func CreateQuote(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/quotes/new", http.StatusSeeOther)
		return
	}

	clientID := parseInt64(r.FormValue("client_id"))
	projectID := parseInt64(r.FormValue("project_id"))
	issueDate := r.FormValue("issue_date")
	validUntil := r.FormValue("valid_until")
	notes := r.FormValue("notes")
	taxRate := parseFloat(r.FormValue("tax_rate"))

	descriptions := r.Form["item_description[]"]
	quantities := r.Form["item_quantity[]"]
	unitPrices := r.Form["item_unit_price[]"]

	quoteNumber := utils.GenerateQuoteNumber()

	var subtotal float64
	for i := range descriptions {
		qty := parseFloat(quantities[i])
		price := parseFloat(unitPrices[i])
		subtotal += qty * price
	}

	taxAmount := subtotal * taxRate / 100
	total := subtotal + taxAmount

	result, err := db.DB.Exec(`INSERT INTO quotes (quote_number, project_id, client_id, issue_date, valid_until, notes, tax_rate, subtotal, tax_amount, total) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, quoteNumber, projectID, clientID, issueDate, validUntil, notes, taxRate, subtotal, taxAmount, total)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	quoteID, _ := result.LastInsertId()

	for i := range descriptions {
		qty := parseFloat(quantities[i])
		price := parseFloat(unitPrices[i])
		amount := qty * price
		db.DB.Exec("INSERT INTO quote_items (quote_id, description, quantity, unit_price, amount) VALUES (?, ?, ?, ?, ?)",
			quoteID, descriptions[i], qty, price, amount)
	}

	http.Redirect(w, r, "/quotes/detail?id="+strconv.FormatInt(quoteID, 10), http.StatusSeeOther)
}

func QuoteDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	id := parseInt64(r.URL.Query().Get("id"))

	var q models.Quote
	err := db.DB.QueryRow(`SELECT q.id, q.quote_number, q.project_id, p.name as project_name, q.client_id, c.company_name,
		q.issue_date, q.valid_until, q.notes, q.status, q.subtotal, q.tax_rate, q.tax_amount, q.total, q.invoice_id
		FROM quotes q LEFT JOIN projects p ON q.project_id = p.id 
		LEFT JOIN clients c ON q.client_id = c.id WHERE q.id = ?`, id).
		Scan(&q.ID, &q.QuoteNumber, &q.ProjectID, &q.ProjectName, &q.ClientID, &q.ClientName,
			&q.IssueDate, &q.ValidUntil, &q.Notes, &q.Status, &q.Subtotal, &q.TaxRate, &q.TaxAmount, &q.Total, &q.InvoiceID)
	if err != nil {
		http.Error(w, "报价单不存在", http.StatusNotFound)
		return
	}

	itemRows, _ := db.DB.Query("SELECT id, description, quantity, unit_price, amount FROM quote_items WHERE quote_id = ?", id)
	for itemRows.Next() {
		var item models.QuoteItem
		itemRows.Scan(&item.ID, &item.Description, &item.Quantity, &item.UnitPrice, &item.Amount)
		q.Items = append(q.Items, item)
	}
	itemRows.Close()

	renderTemplate(w, "quote_detail.html", TemplateData{
		Title:  "报价单详情 - " + q.QuoteNumber,
		User:   user,
		Data:   q,
		Active: "quotes",
	})
}

func UpdateQuoteStatus(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	status := r.URL.Query().Get("status")

	db.DB.Exec("UPDATE quotes SET status = ? WHERE id = ?", status, id)
	http.Redirect(w, r, "/quotes/detail?id="+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func DeleteQuote(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	db.DB.Exec("DELETE FROM quotes WHERE id = ?", id)
	http.Redirect(w, r, "/quotes", http.StatusSeeOther)
}

func ConvertQuoteToInvoice(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))

	var q models.Quote
	db.DB.QueryRow(`SELECT id, quote_number, project_id, client_id, issue_date, notes, tax_rate, subtotal, tax_amount, total 
		FROM quotes WHERE id = ?`, id).Scan(&q.ID, &q.QuoteNumber, &q.ProjectID, &q.ClientID, &q.IssueDate, &q.Notes, &q.TaxRate, &q.Subtotal, &q.TaxAmount, &q.Total)

	itemRows, _ := db.DB.Query("SELECT description, quantity, unit_price, amount FROM quote_items WHERE quote_id = ?", id)
	type quoteItem struct {
		Description string
		Quantity    float64
		UnitPrice   float64
		Amount      float64
	}
	var items []quoteItem
	for itemRows.Next() {
		var item quoteItem
		itemRows.Scan(&item.Description, &item.Quantity, &item.UnitPrice, &item.Amount)
		items = append(items, item)
	}
	itemRows.Close()

	invoiceNumber := utils.GenerateInvoiceNumber()
	issueDate := time.Now().Format("2006-01-02")

	result, _ := db.DB.Exec(`INSERT INTO invoices (invoice_number, project_id, client_id, issue_date, notes, tax_rate, subtotal, tax_amount, total, status) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '已发送')`, invoiceNumber, q.ProjectID, q.ClientID, issueDate, q.Notes, q.TaxRate, q.Subtotal, q.TaxAmount, q.Total)

	invoiceID, _ := result.LastInsertId()

	for _, item := range items {
		db.DB.Exec("INSERT INTO invoice_items (invoice_id, description, quantity, unit_price, amount) VALUES (?, ?, ?, ?, ?)",
			invoiceID, item.Description, item.Quantity, item.UnitPrice, item.Amount)
	}

	db.DB.Exec("UPDATE quotes SET status = '已转发票', invoice_id = ? WHERE id = ?", invoiceID, id)

	http.Redirect(w, r, "/invoices/detail?id="+strconv.FormatInt(invoiceID, 10), http.StatusSeeOther)
}

func SaveQuoteTemplate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/quotes", http.StatusSeeOther)
		return
	}

	name := r.FormValue("name")
	description := r.FormValue("description")
	notes := r.FormValue("notes")
	taxRate := parseFloat(r.FormValue("tax_rate"))

	descriptions := r.Form["item_description[]"]
	quantities := r.Form["item_quantity[]"]
	unitPrices := r.Form["item_unit_price[]"]

	type templateItem struct {
		Description string  `json:"description"`
		Quantity    float64 `json:"quantity"`
		UnitPrice   float64 `json:"unit_price"`
	}

	var items []templateItem
	for i := range descriptions {
		items = append(items, templateItem{
			Description: descriptions[i],
			Quantity:    parseFloat(quantities[i]),
			UnitPrice:   parseFloat(unitPrices[i]),
		})
	}

	itemsJSON, _ := json.Marshal(items)

	db.DB.Exec("INSERT INTO quote_templates (name, description, items, tax_rate, notes) VALUES (?, ?, ?, ?, ?)",
		name, description, string(itemsJSON), taxRate, notes)

	http.Redirect(w, r, "/quotes", http.StatusSeeOther)
}

func DeleteQuoteTemplate(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	db.DB.Exec("DELETE FROM quote_templates WHERE id = ?", id)
	http.Redirect(w, r, "/quotes", http.StatusSeeOther)
}

func GetQuoteTemplate(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))

	var t models.QuoteTemplate
	db.DB.QueryRow("SELECT id, name, description, items, tax_rate, notes FROM quote_templates WHERE id = ?", id).
		Scan(&t.ID, &t.Name, &t.Description, &t.Items, &t.TaxRate, &t.Notes)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}
