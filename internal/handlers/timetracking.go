package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/freelance/workbench/internal/config"
	"github.com/freelance/workbench/internal/db"
	"github.com/freelance/workbench/internal/middleware"
	"github.com/freelance/workbench/internal/models"
	"github.com/freelance/workbench/internal/utils"
)

func TimeTracking(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)

	type entry struct {
		ID              int64
		ProjectID       int64
		ProjectName     string
		ClientName      string
		Description     string
		StartTime       time.Time
		EndTime         *time.Time
		DurationMinutes int
		Rate            float64
		Amount          float64
		Date            string
	}

	var activeTimer *struct {
		ID          int64
		ProjectID   int64
		ProjectName string
		Description string
		StartTime   time.Time
		Elapsed     string
	}

	timerRow := db.DB.QueryRow(`SELECT te.id, te.project_id, p.name, te.description, te.start_time 
		FROM time_entries te LEFT JOIN projects p ON te.project_id = p.id 
		WHERE te.end_time IS NULL LIMIT 1`)
	var t struct {
		ID          int64
		ProjectID   int64
		ProjectName string
		Description string
		StartTime   time.Time
		Elapsed     string
	}
	err := timerRow.Scan(&t.ID, &t.ProjectID, &t.ProjectName, &t.Description, &t.StartTime)
	if err == nil {
		elapsed := time.Since(t.StartTime)
		t.Elapsed = utils.FormatDuration(int(elapsed.Minutes()))
		activeTimer = &t
	}

	dateFilter := r.URL.Query().Get("date")
	projectFilter := r.URL.Query().Get("project_id")

	query := `SELECT te.id, te.project_id, p.name, c.company_name, te.description, te.start_time, te.end_time, te.duration_minutes, te.rate
		FROM time_entries te LEFT JOIN projects p ON te.project_id = p.id 
		LEFT JOIN clients c ON p.client_id = c.id WHERE te.end_time IS NOT NULL`
	var args []interface{}

	if dateFilter != "" {
		query += " AND DATE(te.start_time) = ?"
		args = append(args, dateFilter)
	}
	if projectFilter != "" {
		query += " AND te.project_id = ?"
		args = append(args, parseInt64(projectFilter))
	}

	query += " ORDER BY te.start_time DESC LIMIT 200"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var entries []entry
	for rows.Next() {
		var e entry
		var endTime *time.Time
		rows.Scan(&e.ID, &e.ProjectID, &e.ProjectName, &e.ClientName, &e.Description, &e.StartTime, &endTime, &e.DurationMinutes, &e.Rate)
		e.EndTime = endTime
		e.Date = e.StartTime.Format("2006-01-02")
		e.Amount = float64(e.DurationMinutes) / 60 * e.Rate
		entries = append(entries, e)
	}

	var projects []models.Project
	projRows, _ := db.DB.Query("SELECT id, name FROM projects WHERE status != '已归档' ORDER BY name")
	for projRows.Next() {
		var p models.Project
		projRows.Scan(&p.ID, &p.Name)
		projects = append(projects, p)
	}
	projRows.Close()

	type summary struct {
		Date       string
		Hours      float64
		Amount     float64
	}

	var dailySummary []summary
	if dateFilter == "" {
		startOfWeek := utils.StartOfWeek(time.Now())
		sumRows, _ := db.DB.Query(`SELECT DATE(start_time) as date, SUM(duration_minutes) as mins, SUM(duration_minutes / 60.0 * rate) as amount
			FROM time_entries WHERE end_time IS NOT NULL AND DATE(start_time) >= ?
			GROUP BY DATE(start_time) ORDER BY date DESC`, startOfWeek.Format("2006-01-02"))
		for sumRows.Next() {
			var s summary
			var mins int
			sumRows.Scan(&s.Date, &mins, &s.Amount)
			s.Hours = float64(mins) / 60
			dailySummary = append(dailySummary, s)
		}
		sumRows.Close()
	}

	renderTemplate(w, "time_tracking.html", TemplateData{
		Title:  "工时追踪",
		User:   user,
		Data:   map[string]interface{}{"entries": entries, "activeTimer": activeTimer, "projects": projects, "dateFilter": dateFilter, "projectFilter": projectFilter, "dailySummary": dailySummary, "defaultRate": user.HourlyRate},
		Active: "time",
	})
}

func StartTimer(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/time", http.StatusSeeOther)
		return
	}

	projectID := parseInt64(r.FormValue("project_id"))
	description := r.FormValue("description")
	rate := parseFloat(r.FormValue("rate"))

	user := middleware.GetUser(r)
	if rate == 0 {
		rate = user.HourlyRate
	}

	if rate == 0 {
		var projRate float64
		db.DB.QueryRow("SELECT rate FROM projects WHERE id = ?", projectID).Scan(&projRate)
		rate = projRate
	}

	_, err := db.DB.Exec(`INSERT INTO time_entries (project_id, description, start_time, rate) VALUES (?, ?, ?, ?)`,
		projectID, description, time.Now(), rate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/time", http.StatusSeeOther)
}

func StopTimer(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))

	var startTime time.Time
	var rate float64
	db.DB.QueryRow("SELECT start_time, rate FROM time_entries WHERE id = ?", id).Scan(&startTime, &rate)

	endTime := time.Now()
	duration := int(endTime.Sub(startTime).Minutes())

	db.DB.Exec("UPDATE time_entries SET end_time = ?, duration_minutes = ? WHERE id = ?",
		endTime, duration, id)

	http.Redirect(w, r, "/time", http.StatusSeeOther)
}

func ManualTimeEntry(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/time", http.StatusSeeOther)
		return
	}

	projectID := parseInt64(r.FormValue("project_id"))
	description := r.FormValue("description")
	date := r.FormValue("date")
	startTime := r.FormValue("start_time")
	endTime := r.FormValue("end_time")
	rate := parseFloat(r.FormValue("rate"))

	user := middleware.GetUser(r)
	if rate == 0 {
		rate = user.HourlyRate
	}

	start, _ := time.Parse("2006-01-02 15:04", date+" "+startTime)
	end, _ := time.Parse("2006-01-02 15:04", date+" "+endTime)
	duration := int(end.Sub(start).Minutes())

	_, err := db.DB.Exec(`INSERT INTO time_entries (project_id, description, start_time, end_time, duration_minutes, rate) 
		VALUES (?, ?, ?, ?, ?, ?)`, projectID, description, start, end, duration, rate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/time", http.StatusSeeOther)
}

func DeleteTimeEntry(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	db.DB.Exec("DELETE FROM time_entries WHERE id = ?", id)
	http.Redirect(w, r, "/time", http.StatusSeeOther)
}

func ExportHours(w http.ResponseWriter, r *http.Request) {
	type exportEntry struct {
		Date        string
		Project     string
		Description string
		Duration    float64
		Rate        float64
		Amount      float64
	}

	dateFilter := r.URL.Query().Get("date")
	projectFilter := r.URL.Query().Get("project_id")

	query := `SELECT DATE(te.start_time), p.name, te.description, te.duration_minutes, te.rate
		FROM time_entries te LEFT JOIN projects p ON te.project_id = p.id 
		WHERE te.end_time IS NOT NULL`
	var args []interface{}

	if dateFilter != "" {
		query += " AND DATE(te.start_time) = ?"
		args = append(args, dateFilter)
	}
	if projectFilter != "" {
		query += " AND te.project_id = ?"
		args = append(args, parseInt64(projectFilter))
	}

	query += " ORDER BY te.start_time DESC"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var entries []exportEntry
	for rows.Next() {
		var e exportEntry
		var mins int
		rows.Scan(&e.Date, &e.Project, &e.Description, &mins, &e.Rate)
		e.Duration = float64(mins) / 60
		e.Amount = e.Duration * e.Rate
		entries = append(entries, e)
	}

	type csvEntry struct {
		Date        string
		Project     string
		Description string
		Duration    float64
		Rate        float64
		Amount      float64
	}

	var csvEntries []csvEntry
	for _, e := range entries {
		csvEntries = append(csvEntries, csvEntry{
			Date:        e.Date,
			Project:     e.Project,
			Description: e.Description,
			Duration:    e.Duration,
			Rate:        e.Rate,
			Amount:      e.Amount,
		})
	}

	var records []utils.HourExportEntry
	for _, e := range csvEntries {
		records = append(records, utils.HourExportEntry{
			Date:        e.Date,
			Project:     e.Project,
			Description: e.Description,
			Duration:    e.Duration,
			Rate:        e.Rate,
			Amount:      e.Amount,
		})
	}

	exportDir := config.UploadDir + "/exports"
	filePath, err := utils.ExportHoursToCSV(records, exportDir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Disposition", "attachment; filename=hours_export.csv")
	w.Header().Set("Content-Type", "text/csv")
	http.ServeFile(w, r, filePath)
}

func GetActiveTimerStatus(w http.ResponseWriter, r *http.Request) {
	var timer struct {
		ID          int64  `json:"id"`
		ProjectID   int64  `json:"project_id"`
		ProjectName string `json:"project_name"`
		Description string `json:"description"`
		StartTime   string `json:"start_time"`
		Elapsed     int    `json:"elapsed_seconds"`
	}

	row := db.DB.QueryRow(`SELECT te.id, te.project_id, p.name, te.description, te.start_time 
		FROM time_entries te LEFT JOIN projects p ON te.project_id = p.id 
		WHERE te.end_time IS NULL LIMIT 1`)
	var startTime time.Time
	err := row.Scan(&timer.ID, &timer.ProjectID, &timer.ProjectName, &timer.Description, &startTime)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"active": false})
		return
	}

	timer.StartTime = startTime.Format(time.RFC3339)
	timer.Elapsed = int(time.Since(startTime).Seconds())

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"active": true, "timer": timer})
}

func TimeSummary(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	period := r.URL.Query().Get("period")
	if period == "" {
		period = "week"
	}

	var startDate time.Time
	now := time.Now()

	switch period {
	case "day":
		startDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	case "week":
		startDate = utils.StartOfWeek(now)
	case "month":
		startDate = utils.StartOfMonth(now)
	default:
		startDate = utils.StartOfWeek(now)
	}

	type projSummary struct {
		ProjectID   int64
		ProjectName string
		Hours       float64
		Amount      float64
	}

	rows, _ := db.DB.Query(`SELECT p.id, p.name, SUM(te.duration_minutes) as mins, SUM(te.duration_minutes / 60.0 * te.rate) as amount
		FROM time_entries te LEFT JOIN projects p ON te.project_id = p.id 
		WHERE te.end_time IS NOT NULL AND te.start_time >= ?
		GROUP BY p.id, p.name ORDER BY amount DESC`, startDate)
	var summaries []projSummary
	for rows.Next() {
		var s projSummary
		var mins int
		rows.Scan(&s.ProjectID, &s.ProjectName, &mins, &s.Amount)
		s.Hours = float64(mins) / 60
		summaries = append(summaries, s)
	}
	rows.Close()

	var totalHours float64
	var totalAmount float64
	for _, s := range summaries {
		totalHours += s.Hours
		totalAmount += s.Amount
	}

	avgRate := 0.0
	if totalHours > 0 {
		avgRate = totalAmount / totalHours
	}

	type response struct {
		Period      string
		StartDate   string
		TotalHours  float64
		TotalAmount float64
		AvgRate     float64
		Currency    string
		Projects    []projSummary
	}

	resp := response{
		Period:      period,
		StartDate:   startDate.Format("2006-01-02"),
		TotalHours:  totalHours,
		TotalAmount: totalAmount,
		AvgRate:     avgRate,
		Currency:    user.Currency,
		Projects:    summaries,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
