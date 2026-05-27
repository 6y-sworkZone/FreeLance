package utils

import (
	"database/sql"
	"fmt"
	"time"
)

func CheckAndCreateNotifications(db *sql.DB, userID int64) error {
	now := time.Now()
	today := now.Format("2006-01-02")

	rows, err := db.Query(`SELECT i.id, i.invoice_number, c.company_name, i.due_date, i.total, i.paid_amount
		FROM invoices i LEFT JOIN clients c ON i.client_id = c.id 
		WHERE i.status IN ('已发送', '已逾期') AND i.total > i.paid_amount AND i.due_date <= ?
		AND NOT EXISTS (SELECT 1 FROM notifications WHERE related_type = 'invoice' AND related_id = i.id AND is_read = 0)`,
		today)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var invNum, clientName, dueDate string
		var total, paidAmount float64
		rows.Scan(&id, &invNum, &clientName, &dueDate, &total, &paidAmount)
		unpaid := total - paidAmount

		isOverdue := dueDate < today
		var title, content string
		if isOverdue {
			title = "发票已逾期"
			content = fmt.Sprintf("发票 %s（客户：%s）已逾期，未收金额 ¥%.0f", invNum, clientName, unpaid)
		} else {
			title = "发票今日到期"
			content = fmt.Sprintf("发票 %s（客户：%s）今日到期，未收金额 ¥%.0f", invNum, clientName, unpaid)
		}

		db.Exec(`INSERT INTO notifications (user_id, type, title, content, related_type, related_id, due_date)
			VALUES (?, 'invoice', ?, ?, 'invoice', ?, ?)`,
			userID, title, content, id, dueDate)
	}
	rows.Close()

	msRows, err := db.Query(`SELECT m.id, m.title, p.name, m.due_date
		FROM milestones m LEFT JOIN projects p ON m.project_id = p.id 
		WHERE m.status != '已完成' AND m.due_date <= ? AND m.due_date != ''
		AND NOT EXISTS (SELECT 1 FROM notifications WHERE related_type = 'milestone' AND related_id = m.id AND is_read = 0)`,
		today)
	if err != nil {
		return err
	}
	defer msRows.Close()

	for msRows.Next() {
		var id int64
		var title, projectName, dueDate string
		msRows.Scan(&id, &title, &projectName, &dueDate)

		isOverdue := dueDate < today
		var notifTitle, content string
		if isOverdue {
			notifTitle = "里程碑已逾期"
			content = fmt.Sprintf("项目 '%s' 的里程碑 '%s' 已逾期", projectName, title)
		} else {
			notifTitle = "里程碑今日到期"
			content = fmt.Sprintf("项目 '%s' 的里程碑 '%s' 今日到期", projectName, title)
		}

		db.Exec(`INSERT INTO notifications (user_id, type, title, content, related_type, related_id, due_date)
			VALUES (?, 'milestone', ?, ?, 'milestone', ?, ?)`,
			userID, notifTitle, content, id, dueDate)
	}
	msRows.Close()

	return nil
}
