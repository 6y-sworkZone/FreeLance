package models

import "time"

type User struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	Password    string    `json:"-"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	HourlyRate  float64   `json:"hourly_rate"`
	Currency    string    `json:"currency"`
	TaxRate     float64   `json:"tax_rate"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Client struct {
	ID             int64     `json:"id"`
	CompanyName    string    `json:"company_name"`
	ContactPerson  string    `json:"contact_person"`
	Email          string    `json:"email"`
	Phone          string    `json:"phone"`
	Industry       string    `json:"industry"`
	Source         string    `json:"source"`
	Notes          string    `json:"notes"`
	ValueScore     float64   `json:"value_score"`
	Tags           []Tag     `json:"tags,omitempty"`
	ProjectCount   int       `json:"project_count,omitempty"`
	TotalRevenue   float64   `json:"total_revenue,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Tag struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Communication struct {
	ID          int64     `json:"id"`
	ClientID    int64     `json:"client_id"`
	Type        string    `json:"type"`
	Content     string    `json:"content"`
	OccurredAt  time.Time `json:"occurred_at"`
	CreatedAt   time.Time `json:"created_at"`
}

type Project struct {
	ID             int64     `json:"id"`
	ClientID       int64     `json:"client_id"`
	ClientName     string    `json:"client_name,omitempty"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Type           string    `json:"type"`
	Status         string    `json:"status"`
	Rate           float64   `json:"rate"`
	EstimatedHours float64   `json:"estimated_hours"`
	Budget         float64   `json:"budget"`
	StartDate      string    `json:"start_date"`
	DueDate        string    `json:"due_date"`
	Tags           []Tag     `json:"tags,omitempty"`
	TotalHours     float64   `json:"total_hours,omitempty"`
	TotalAmount    float64   `json:"total_amount,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Milestone struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"project_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	DueDate     string    `json:"due_date"`
	Deliverable string    `json:"deliverable"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type ProjectFile struct {
	ID           int64     `json:"id"`
	ProjectID    int64     `json:"project_id"`
	Filename     string    `json:"filename"`
	OriginalName string    `json:"original_name"`
	Size         int64     `json:"size"`
	FileType     string    `json:"file_type"`
	Category     string    `json:"category"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

type TimeEntry struct {
	ID              int64     `json:"id"`
	ProjectID       int64     `json:"project_id"`
	ProjectName     string    `json:"project_name,omitempty"`
	Description     string    `json:"description"`
	StartTime       time.Time `json:"start_time"`
	EndTime         *time.Time `json:"end_time,omitempty"`
	DurationMinutes int       `json:"duration_minutes"`
	Rate            float64   `json:"rate"`
	CreatedAt       time.Time `json:"created_at"`
}

type Invoice struct {
	ID            int64          `json:"id"`
	InvoiceNumber string         `json:"invoice_number"`
	ProjectID     int64          `json:"project_id"`
	ProjectName   string         `json:"project_name,omitempty"`
	ClientID      int64          `json:"client_id"`
	ClientName    string         `json:"client_name,omitempty"`
	IssueDate     string         `json:"issue_date"`
	DueDate       string         `json:"due_date"`
	Notes         string         `json:"notes"`
	Status        string         `json:"status"`
	Subtotal      float64        `json:"subtotal"`
	TaxRate       float64        `json:"tax_rate"`
	TaxAmount     float64        `json:"tax_amount"`
	Total         float64        `json:"total"`
	PaidAmount    float64        `json:"paid_amount"`
	Items         []InvoiceItem  `json:"items,omitempty"`
	Payments      []Payment      `json:"payments,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

type InvoiceItem struct {
	ID          int64   `json:"id"`
	InvoiceID   int64   `json:"invoice_id"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Amount      float64 `json:"amount"`
}

type Payment struct {
	ID          int64     `json:"id"`
	InvoiceID   int64     `json:"invoice_id"`
	Amount      float64   `json:"amount"`
	PaymentDate string    `json:"payment_date"`
	Method      string    `json:"method"`
	Notes       string    `json:"notes"`
	CreatedAt   time.Time `json:"created_at"`
}

type Quote struct {
	ID            int64       `json:"id"`
	QuoteNumber   string      `json:"quote_number"`
	ProjectID     int64       `json:"project_id"`
	ProjectName   string      `json:"project_name,omitempty"`
	ClientID      int64       `json:"client_id"`
	ClientName    string      `json:"client_name,omitempty"`
	IssueDate     string      `json:"issue_date"`
	ValidUntil    string      `json:"valid_until"`
	Notes         string      `json:"notes"`
	Status        string      `json:"status"`
	Subtotal      float64     `json:"subtotal"`
	TaxRate       float64     `json:"tax_rate"`
	TaxAmount     float64     `json:"tax_amount"`
	Total         float64     `json:"total"`
	InvoiceID     int64       `json:"invoice_id"`
	Items         []QuoteItem `json:"items,omitempty"`
	CreatedAt     time.Time   `json:"created_at"`
	UpdatedAt     time.Time   `json:"updated_at"`
}

type QuoteItem struct {
	ID          int64   `json:"id"`
	QuoteID     int64   `json:"quote_id"`
	Description string  `json:"description"`
	Quantity    float64 `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Amount      float64 `json:"amount"`
}

type QuoteTemplate struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Items       string    `json:"items"`
	TaxRate     float64   `json:"tax_rate"`
	Notes       string    `json:"notes"`
	CreatedAt   time.Time `json:"created_at"`
}

type Session struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type DashboardStats struct {
	TodayHours       float64 `json:"today_hours"`
	WeekHours        float64 `json:"week_hours"`
	MonthRevenue     float64 `json:"month_revenue"`
	UnpaidInvoices   float64 `json:"unpaid_invoices"`
	OverdueInvoices  int     `json:"overdue_invoices"`
	ActiveProjects   int     `json:"active_projects"`
}

type ActivityItem struct {
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Detail    string    `json:"detail"`
	Time      time.Time `json:"time"`
}

type Notification struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	RelatedType string    `json:"related_type"`
	RelatedID   int64     `json:"related_id"`
	IsRead      bool      `json:"is_read"`
	DueDate     string    `json:"due_date"`
	CreatedAt   time.Time `json:"created_at"`
}
