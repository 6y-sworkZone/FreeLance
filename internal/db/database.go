package db

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func Init(dbPath string) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	var err error
	DB, err = sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}

	DB.SetMaxOpenConns(1)
	if err = DB.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	createTables()
	seedData()
}

func createTables() {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			name TEXT NOT NULL,
			email TEXT,
			hourly_rate REAL DEFAULT 0,
			currency TEXT DEFAULT 'CNY',
			tax_rate REAL DEFAULT 0.06,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS clients (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			company_name TEXT NOT NULL,
			contact_person TEXT,
			email TEXT,
			phone TEXT,
			industry TEXT,
			source TEXT DEFAULT '主动联系',
			notes TEXT,
			value_score REAL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS client_tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			color TEXT DEFAULT '#3b82f6',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS client_tag_maps (
			client_id INTEGER NOT NULL,
			tag_id INTEGER NOT NULL,
			PRIMARY KEY (client_id, tag_id),
			FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE,
			FOREIGN KEY (tag_id) REFERENCES client_tags(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS communications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			client_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			content TEXT NOT NULL,
			occurred_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			client_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			description TEXT,
			type TEXT DEFAULT '开发',
			status TEXT DEFAULT '洽谈中',
			rate REAL DEFAULT 0,
			estimated_hours REAL DEFAULT 0,
			budget REAL DEFAULT 0,
			start_date TEXT,
			due_date TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS project_tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			color TEXT DEFAULT '#10b981',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS project_tag_maps (
			project_id INTEGER NOT NULL,
			tag_id INTEGER NOT NULL,
			PRIMARY KEY (project_id, tag_id),
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
			FOREIGN KEY (tag_id) REFERENCES project_tags(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS milestones (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			due_date TEXT,
			deliverable TEXT,
			status TEXT DEFAULT '未开始',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS project_files (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			filename TEXT NOT NULL,
			original_name TEXT NOT NULL,
			size INTEGER DEFAULT 0,
			file_type TEXT,
			category TEXT DEFAULT '其他',
			uploaded_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS time_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL,
			description TEXT,
			start_time DATETIME NOT NULL,
			end_time DATETIME,
			duration_minutes INTEGER DEFAULT 0,
			rate REAL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS invoices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			invoice_number TEXT NOT NULL UNIQUE,
			project_id INTEGER NOT NULL,
			client_id INTEGER NOT NULL,
			issue_date TEXT NOT NULL,
			due_date TEXT,
			notes TEXT,
			status TEXT DEFAULT '草稿',
			subtotal REAL DEFAULT 0,
			tax_rate REAL DEFAULT 0,
			tax_amount REAL DEFAULT 0,
			total REAL DEFAULT 0,
			paid_amount REAL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id),
			FOREIGN KEY (client_id) REFERENCES clients(id)
		)`,
		`CREATE TABLE IF NOT EXISTS invoice_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			invoice_id INTEGER NOT NULL,
			description TEXT NOT NULL,
			quantity REAL DEFAULT 1,
			unit_price REAL DEFAULT 0,
			amount REAL DEFAULT 0,
			FOREIGN KEY (invoice_id) REFERENCES invoices(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS payments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			invoice_id INTEGER NOT NULL,
			amount REAL NOT NULL,
			payment_date TEXT NOT NULL,
			method TEXT DEFAULT '银行转账',
			notes TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (invoice_id) REFERENCES invoices(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS quotes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			quote_number TEXT NOT NULL UNIQUE,
			project_id INTEGER,
			client_id INTEGER NOT NULL,
			issue_date TEXT NOT NULL,
			valid_until TEXT,
			notes TEXT,
			status TEXT DEFAULT '草稿',
			subtotal REAL DEFAULT 0,
			tax_rate REAL DEFAULT 0,
			tax_amount REAL DEFAULT 0,
			total REAL DEFAULT 0,
			invoice_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (project_id) REFERENCES projects(id),
			FOREIGN KEY (client_id) REFERENCES clients(id),
			FOREIGN KEY (invoice_id) REFERENCES invoices(id)
		)`,
		`CREATE TABLE IF NOT EXISTS quote_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			quote_id INTEGER NOT NULL,
			description TEXT NOT NULL,
			quantity REAL DEFAULT 1,
			unit_price REAL DEFAULT 0,
			amount REAL DEFAULT 0,
			FOREIGN KEY (quote_id) REFERENCES quotes(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS quote_templates (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT,
			items TEXT NOT NULL,
			tax_rate REAL DEFAULT 0,
			notes TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT NOT NULL,
			related_type TEXT,
			related_id INTEGER,
			is_read BOOLEAN DEFAULT 0,
			due_date TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
	}

	for _, stmt := range statements {
		if _, err := DB.Exec(stmt); err != nil {
			log.Printf("Warning: %v", err)
		}
	}
}

func seedData() {
	var count int
	DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count > 0 {
		return
	}

	_, err := DB.Exec(`INSERT INTO users (username, password, name, email, hourly_rate, currency, tax_rate)
		VALUES ('admin', '$2a$10$rMR2GV7SajhlAYaeDMmDU.GHyWjWUcV4ZfOsErFeMvK/ChMG0d5Fe', '管理员', 'admin@example.com', 200, 'CNY', 0.06)`)
	if err != nil {
		log.Printf("Warning creating default user: %v", err)
	}

	defaultTags := []struct {
		name  string
		color string
	}{
		{"重要", "#ef4444"},
		{"长期合作", "#3b82f6"},
		{"潜力客户", "#f59e0b"},
		{"已合作", "#10b981"},
		{"待跟进", "#8b5cf6"},
	}

	for _, t := range defaultTags {
		DB.Exec("INSERT OR IGNORE INTO client_tags (name, color) VALUES (?, ?)", t.name, t.color)
	}

	defaultProjTags := []struct {
		name  string
		color string
	}{
		{"紧急", "#ef4444"},
		{"高优先级", "#f59e0b"},
		{"设计", "#3b82f6"},
		{"开发", "#10b981"},
		{"咨询", "#8b5cf6"},
	}

	for _, t := range defaultProjTags {
		DB.Exec("INSERT OR IGNORE INTO project_tags (name, color) VALUES (?, ?)", t.name, t.color)
	}
}
