package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/freelance/workbench/internal/db"
	"github.com/freelance/workbench/internal/middleware"
	"github.com/freelance/workbench/internal/models"
	"github.com/freelance/workbench/internal/utils"
)

type activity struct {
	Type   string
	Title  string
	Detail string
	Time   time.Time
}

func Home(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	now := time.Now()

	utils.CheckAndCreateNotifications(db.DB, user.ID)

	var notifications []models.Notification
	notifRows, _ := db.DB.Query(`SELECT id, type, title, content, related_type, related_id, is_read, due_date, created_at
		FROM notifications WHERE user_id = ? AND is_read = 0 ORDER BY created_at DESC LIMIT 10`, user.ID)
	for notifRows.Next() {
		var n models.Notification
		notifRows.Scan(&n.ID, &n.Type, &n.Title, &n.Content, &n.RelatedType, &n.RelatedID, &n.IsRead, &n.DueDate, &n.CreatedAt)
		notifications = append(notifications, n)
	}
	notifRows.Close()

	var unreadCount int
	db.DB.QueryRow("SELECT COUNT(*) FROM notifications WHERE user_id = ? AND is_read = 0", user.ID).Scan(&unreadCount)

	var activeTimer *models.TimeEntry
	timerRow := db.DB.QueryRow(`SELECT te.id, te.project_id, p.name, te.description, te.start_time 
		FROM time_entries te LEFT JOIN projects p ON te.project_id = p.id 
		WHERE te.end_time IS NULL LIMIT 1`)
	var t models.TimeEntry
	err := timerRow.Scan(&t.ID, &t.ProjectID, &t.ProjectName, &t.Description, &t.StartTime)
	if err == nil {
		activeTimer = &t
	}

	var todayMinutes int
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	db.DB.QueryRow("SELECT COALESCE(SUM(duration_minutes), 0) FROM time_entries WHERE end_time IS NOT NULL AND start_time >= ?",
		todayStart).Scan(&todayMinutes)

	var weekMinutes int
	weekStart := utils.StartOfWeek(now)
	db.DB.QueryRow("SELECT COALESCE(SUM(duration_minutes), 0) FROM time_entries WHERE end_time IS NOT NULL AND start_time >= ?",
		weekStart).Scan(&weekMinutes)

	var monthRevenue float64
	monthStart := utils.StartOfMonth(now)
	db.DB.QueryRow("SELECT COALESCE(SUM(total), 0) FROM invoices WHERE status = '已支付' AND issue_date >= ?",
		monthStart.Format("2006-01-02")).Scan(&monthRevenue)

	var unpaidTotal float64
	var overdueCount int
	db.DB.QueryRow("SELECT COALESCE(SUM(total - paid_amount), 0) FROM invoices WHERE status IN ('已发送', '已逾期') AND total > paid_amount").
		Scan(&unpaidTotal)
	db.DB.QueryRow("SELECT COUNT(*) FROM invoices WHERE status = '已逾期'").Scan(&overdueCount)

	var activeProjects int
	db.DB.QueryRow("SELECT COUNT(*) FROM projects WHERE status = '进行中'").Scan(&activeProjects)

	var overdueMilestones []struct {
		ID        int64
		Title     string
		Project   string
		DueDate   string
	}
	milestoneRows, _ := db.DB.Query(`SELECT m.id, m.title, p.name, m.due_date 
		FROM milestones m LEFT JOIN projects p ON m.project_id = p.id 
		WHERE m.status != '已完成' AND m.due_date <= ? AND m.due_date != ''
		ORDER BY m.due_date ASC LIMIT 10`, now.Format("2006-01-02"))
	for milestoneRows.Next() {
		var ms struct {
			ID      int64
			Title   string
			Project string
			DueDate string
		}
		milestoneRows.Scan(&ms.ID, &ms.Title, &ms.Project, &ms.DueDate)
		overdueMilestones = append(overdueMilestones, ms)
	}
	milestoneRows.Close()

	var todayMilestones []struct {
		ID      int64
		Title   string
		Project string
		DueDate string
	}
	todayMsRows, _ := db.DB.Query(`SELECT m.id, m.title, p.name, m.due_date 
		FROM milestones m LEFT JOIN projects p ON m.project_id = p.id 
		WHERE m.status != '已完成' AND DATE(m.due_date) = ?
		ORDER BY m.due_date ASC`, now.Format("2006-01-02"))
	for todayMsRows.Next() {
		var ms struct {
			ID      int64
			Title   string
			Project string
			DueDate string
		}
		todayMsRows.Scan(&ms.ID, &ms.Title, &ms.Project, &ms.DueDate)
		todayMilestones = append(todayMilestones, ms)
	}
	todayMsRows.Close()

	var unsentInvoices []struct {
		ID            int64
		InvoiceNumber string
		ClientName    string
		Total         float64
		IssueDate     string
	}
	invoiceRows, _ := db.DB.Query(`SELECT i.id, i.invoice_number, c.company_name, i.total, i.issue_date 
		FROM invoices i LEFT JOIN clients c ON i.client_id = c.id 
		WHERE i.status = '草稿' ORDER BY i.created_at DESC LIMIT 5`)
	for invoiceRows.Next() {
		var inv struct {
			ID            int64
			InvoiceNumber string
			ClientName    string
			Total         float64
			IssueDate     string
		}
		invoiceRows.Scan(&inv.ID, &inv.InvoiceNumber, &inv.ClientName, &inv.Total, &inv.IssueDate)
		unsentInvoices = append(unsentInvoices, inv)
	}
	invoiceRows.Close()

	var pendingClients []struct {
		ID           int64
		CompanyName  string
		LastContact  string
	}
	clientRows, _ := db.DB.Query(`SELECT c.id, c.company_name, MAX(com.occurred_at) as last_contact 
		FROM clients c LEFT JOIN communications com ON c.id = com.client_id 
		WHERE c.id IN (SELECT DISTINCT client_id FROM projects WHERE status IN ('洽谈中', '进行中'))
		GROUP BY c.id HAVING last_contact IS NULL OR last_contact < ?
		ORDER BY last_contact ASC LIMIT 5`, now.AddDate(-0, -1, 0).Format("2006-01-02"))
	for clientRows.Next() {
		var c struct {
			ID          int64
			CompanyName string
			LastContact string
		}
		clientRows.Scan(&c.ID, &c.CompanyName, &c.LastContact)
		pendingClients = append(pendingClients, c)
	}
	clientRows.Close()

	var activities []activity

	timeRows, _ := db.DB.Query(`SELECT te.start_time, p.name, te.description 
		FROM time_entries te LEFT JOIN projects p ON te.project_id = p.id 
		WHERE te.end_time IS NOT NULL ORDER BY te.created_at DESC LIMIT 5`)
	for timeRows.Next() {
		var startTime time.Time
		var projName, desc string
		timeRows.Scan(&startTime, &projName, &desc)
		activities = append(activities, activity{
			Type:   "hour",
			Title:  "工时记录",
			Detail: projName + " - " + desc,
			Time:   startTime,
		})
	}
	timeRows.Close()

	payRows, _ := db.DB.Query(`SELECT p.created_at, i.invoice_number, c.company_name, p.amount 
		FROM payments p LEFT JOIN invoices i ON p.invoice_id = i.id 
		LEFT JOIN clients c ON i.client_id = c.id 
		ORDER BY p.created_at DESC LIMIT 5`)
	for payRows.Next() {
		var createdAt time.Time
		var invNum, clientName string
		var amount float64
		payRows.Scan(&createdAt, &invNum, &clientName, &amount)
		activities = append(activities, activity{
			Type:   "payment",
			Title:  "收到付款",
			Detail: clientName + " - " + invNum + " (¥" + strconv.FormatFloat(amount, 'f', 2, 64) + ")",
			Time:   createdAt,
		})
	}
	payRows.Close()

	commRows, _ := db.DB.Query(`SELECT com.created_at, com.type, com.content, c.company_name 
		FROM communications com LEFT JOIN clients c ON com.client_id = c.id 
		ORDER BY com.created_at DESC LIMIT 5`)
	for commRows.Next() {
		var createdAt time.Time
		var commType, content, clientName string
		commRows.Scan(&createdAt, &commType, &content, &clientName)
		activities = append(activities, activity{
			Type:   "communication",
			Title:  "沟通记录 - " + clientName,
			Detail: commType + ": " + content,
			Time:   createdAt,
		})
	}
	commRows.Close()

	sortActivities(activities)

	var projects []models.Project
	projRows, _ := db.DB.Query("SELECT id, name, status, type, due_date FROM projects WHERE status != '已归档' ORDER BY created_at DESC LIMIT 10")
	for projRows.Next() {
		var p models.Project
		projRows.Scan(&p.ID, &p.Name, &p.Status, &p.Type, &p.DueDate)
		projects = append(projects, p)
	}
	projRows.Close()

	renderTemplate(w, "home.html", TemplateData{
		Title:  "工作台",
		User:   user,
		Data: map[string]interface{}{
			"todayHours":        float64(todayMinutes) / 60,
			"weekHours":         float64(weekMinutes) / 60,
			"monthRevenue":      monthRevenue,
			"unpaidTotal":       unpaidTotal,
			"overdueCount":      overdueCount,
			"activeProjects":   activeProjects,
			"activeTimer":       activeTimer,
			"overdueMilestones": overdueMilestones,
			"todayMilestones":   todayMilestones,
			"unsentInvoices":    unsentInvoices,
			"pendingClients":    pendingClients,
			"activities":        activities,
			"projects":          projects,
			"notifications":     notifications,
			"unreadCount":       unreadCount,
		},
		Active: "home",
	})
}

func sortActivities(activities []activity) {
	for i := 0; i < len(activities); i++ {
		for j := i + 1; j < len(activities); j++ {
			if activities[j].Time.After(activities[i].Time) {
				activities[i], activities[j] = activities[j], activities[i]
			}
		}
	}
}

func MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	db.DB.Exec("UPDATE notifications SET is_read = 1 WHERE id = ?", id)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	db.DB.Exec("UPDATE notifications SET is_read = 1 WHERE user_id = ?", user.ID)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}
