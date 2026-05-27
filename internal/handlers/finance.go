package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/freelance/workbench/internal/db"
	"github.com/freelance/workbench/internal/middleware"
	"github.com/freelance/workbench/internal/utils"
)

func FinanceDashboard(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	year := r.URL.Query().Get("year")
	if year == "" {
		year = time.Now().Format("2006")
	}

	type monthData struct {
		Month  string  `json:"month"`
		Income float64 `json:"income"`
		Expense float64 `json:"expense"`
	}

	var monthlyData []monthData
	startDate := year + "-01-01"
	endDate := year + "-12-31"

	rows, _ := db.DB.Query(`SELECT strftime('%Y-%m', i.issue_date) as month, COALESCE(SUM(i.total), 0) as income
		FROM invoices i WHERE i.status = '已支付' AND i.issue_date >= ? AND i.issue_date <= ?
		GROUP BY strftime('%Y-%m', i.issue_date) ORDER BY month`, startDate, endDate)
	for rows.Next() {
		var m monthData
		rows.Scan(&m.Month, &m.Income)
		monthlyData = append(monthlyData, m)
	}
	rows.Close()

	var totalRevenue float64
	db.DB.QueryRow(`SELECT COALESCE(SUM(total), 0) FROM invoices WHERE status = '已支付' AND strftime('%Y', issue_date) = ?`, year).
		Scan(&totalRevenue)

	var unpaidTotal float64
	var unpaidCount int
	unpaidRows, _ := db.DB.Query(`SELECT i.id, i.invoice_number, i.total, i.paid_amount, i.issue_date, i.due_date, i.status, c.company_name
		FROM invoices i LEFT JOIN clients c ON i.client_id = c.id 
		WHERE i.status IN ('已发送', '已逾期') AND i.total > i.paid_amount
		ORDER BY i.due_date ASC`)

	type unpaidInvoice struct {
		ID            int64   `json:"id"`
		InvoiceNumber string  `json:"invoice_number"`
		ClientName    string  `json:"client_name"`
		IssueDate     string  `json:"issue_date"`
		DueDate       string  `json:"due_date"`
		Total         float64 `json:"total"`
		PaidAmount    float64 `json:"paid_amount"`
		Unpaid        float64 `json:"unpaid"`
		Status        string  `json:"status"`
		DaysOverdue   int     `json:"days_overdue"`
	}

	var unpaidInvoices []unpaidInvoice
	for unpaidRows.Next() {
		var inv unpaidInvoice
		unpaidRows.Scan(&inv.ID, &inv.InvoiceNumber, &inv.Total, &inv.PaidAmount, &inv.IssueDate, &inv.DueDate, &inv.Status, &inv.ClientName)
		inv.Unpaid = inv.Total - inv.PaidAmount
		unpaidTotal += inv.Unpaid
		unpaidCount++

		if inv.DueDate != "" {
			due, _ := time.Parse("2006-01-02", inv.DueDate)
			if due.Before(time.Now()) {
				inv.DaysOverdue = utils.DaysBetween(due, time.Now())
			}
		}
		unpaidInvoices = append(unpaidInvoices, inv)
	}
	unpaidRows.Close()

	type incomeSource struct {
		ClientName string  `json:"client_name"`
		Amount     float64 `json:"amount"`
		Percentage float64 `json:"percentage"`
	}

	var incomeSources []incomeSource
	sourceRows, _ := db.DB.Query(`SELECT c.company_name, COALESCE(SUM(i.total), 0) as amount
		FROM invoices i LEFT JOIN clients c ON i.client_id = c.id 
		WHERE i.status = '已支付' AND strftime('%Y', i.issue_date) = ?
		GROUP BY c.company_name ORDER BY amount DESC`, year)
	for sourceRows.Next() {
		var s incomeSource
		sourceRows.Scan(&s.ClientName, &s.Amount)
		if totalRevenue > 0 {
			s.Percentage = s.Amount / totalRevenue * 100
		}
		incomeSources = append(incomeSources, s)
	}
	sourceRows.Close()

	type projectTypeIncome struct {
		Type   string  `json:"type"`
		Amount float64 `json:"amount"`
	}

	var typeIncomes []projectTypeIncome
	typeRows, _ := db.DB.Query(`SELECT p.type, COALESCE(SUM(i.total), 0) as amount
		FROM invoices i LEFT JOIN projects p ON i.project_id = p.id 
		WHERE i.status = '已支付' AND strftime('%Y', i.issue_date) = ?
		GROUP BY p.type ORDER BY amount DESC`, year)
	for typeRows.Next() {
		var t projectTypeIncome
		typeRows.Scan(&t.Type, &t.Amount)
		typeIncomes = append(typeIncomes, t)
	}
	typeRows.Close()

	var lastMonthRevenue float64
	var lastYearRevenue float64
	currentMonth := time.Now().Format("2006-01")
	lastMonth := time.Now().AddDate(0, -1, 0).Format("2006-01")
	lastYear := strconv.Itoa(time.Now().Year() - 1)

	db.DB.QueryRow(`SELECT COALESCE(SUM(total), 0) FROM invoices WHERE status = '已支付' AND strftime('%Y-%m', issue_date) = ?`, lastMonth).
		Scan(&lastMonthRevenue)
	db.DB.QueryRow(`SELECT COALESCE(SUM(total), 0) FROM invoices WHERE status = '已支付' AND strftime('%Y', issue_date) = ?`, lastYear).
		Scan(&lastYearRevenue)

	var currentMonthRevenue float64
	db.DB.QueryRow(`SELECT COALESCE(SUM(total), 0) FROM invoices WHERE status = '已支付' AND strftime('%Y-%m', issue_date) = ?`, currentMonth).
		Scan(&currentMonthRevenue)

	monthlyGrowth := 0.0
	if lastMonthRevenue > 0 {
		monthlyGrowth = (currentMonthRevenue - lastMonthRevenue) / lastMonthRevenue * 100
	}

	yearlyGrowth := 0.0
	if lastYearRevenue > 0 {
		yearlyGrowth = (totalRevenue - lastYearRevenue) / lastYearRevenue * 100
	}

	taxEstimate := totalRevenue * user.TaxRate / 100
	taxableIncome := totalRevenue * 0.7
	estimatedTax := taxableIncome * user.TaxRate / 100

	type cashflowItem struct {
		Date        string  `json:"date"`
		Amount      float64 `json:"amount"`
		Type        string  `json:"type"`
		Description string  `json:"description"`
	}

	var cashflow []cashflowItem
	futureRows, _ := db.DB.Query(`SELECT i.due_date, (i.total - i.paid_amount) as amount, c.company_name
		FROM invoices i LEFT JOIN clients c ON i.client_id = c.id 
		WHERE i.status IN ('已发送', '已逾期') AND i.due_date >= ?
		ORDER BY i.due_date ASC`, time.Now().Format("2006-01-02"))
	for futureRows.Next() {
		var c cashflowItem
		var amount float64
		futureRows.Scan(&c.Date, &amount, &c.Description)
		c.Amount = amount
		c.Type = "收入"
		cashflow = append(cashflow, c)
	}
	futureRows.Close()

	projectRows, _ := db.DB.Query(`SELECT p.name, p.budget, p.due_date FROM projects p 
		WHERE p.status = '进行中' AND p.due_date >= ? ORDER BY p.due_date ASC`, time.Now().Format("2006-01-02"))
	for projectRows.Next() {
		var c cashflowItem
		var name string
		var budget float64
		var dueDate string
		projectRows.Scan(&name, &budget, &dueDate)
		c.Date = dueDate
		c.Amount = budget
		c.Type = "预估"
		c.Description = name
		cashflow = append(cashflow, c)
	}
	projectRows.Close()

	monthlyJSON, _ := json.Marshal(monthlyData)

	renderTemplate(w, "finance.html", TemplateData{
		Title:  "财务看板",
		User:   user,
		Data: map[string]interface{}{
			"year":              year,
			"totalRevenue":      totalRevenue,
			"monthlyData":       string(monthlyJSON),
			"unpaidInvoices":    unpaidInvoices,
			"unpaidTotal":       unpaidTotal,
			"unpaidCount":       unpaidCount,
			"incomeSources":     incomeSources,
			"typeIncomes":       typeIncomes,
			"currentMonthRevenue": currentMonthRevenue,
			"lastMonthRevenue":  lastMonthRevenue,
			"lastYearRevenue":   lastYearRevenue,
			"monthlyGrowth":     monthlyGrowth,
			"yearlyGrowth":      yearlyGrowth,
			"taxEstimate":       taxEstimate,
			"taxableIncome":     taxableIncome,
			"estimatedTax":      estimatedTax,
			"cashflow":          cashflow,
		},
		Active: "finance",
	})
}
