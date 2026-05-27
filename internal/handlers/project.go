package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/freelance/workbench/internal/config"
	"github.com/freelance/workbench/internal/db"
	"github.com/freelance/workbench/internal/middleware"
	"github.com/freelance/workbench/internal/models"
)

func ListProjects(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	search := r.URL.Query().Get("search")
	statusFilter := r.URL.Query().Get("status")
	typeFilter := r.URL.Query().Get("type")
	tagFilter := r.URL.Query().Get("tag")

	query := `SELECT p.id, p.client_id, c.company_name, p.name, p.description, p.type, p.status, p.rate, p.estimated_hours, p.budget, p.start_date, p.due_date, p.created_at, p.updated_at
		FROM projects p LEFT JOIN clients c ON p.client_id = c.id WHERE 1=1`
	var args []interface{}

	if search != "" {
		query += " AND (p.name LIKE ? OR p.description LIKE ?)"
		searchLike := "%" + search + "%"
		args = append(args, searchLike, searchLike)
	}
	if statusFilter != "" {
		query += " AND p.status = ?"
		args = append(args, statusFilter)
	}
	if typeFilter != "" {
		query += " AND p.type = ?"
		args = append(args, typeFilter)
	}
	if tagFilter != "" {
		query += " AND p.id IN (SELECT project_id FROM project_tag_maps WHERE tag_id = ?)"
		args = append(args, parseInt64(tagFilter))
	}

	query += " ORDER BY p.created_at DESC"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		rows.Scan(&p.ID, &p.ClientID, &p.ClientName, &p.Name, &p.Description, &p.Type, &p.Status,
			&p.Rate, &p.EstimatedHours, &p.Budget, &p.StartDate, &p.DueDate, &p.CreatedAt, &p.UpdatedAt)

		var totalMinutes int
		db.DB.QueryRow("SELECT COALESCE(SUM(duration_minutes), 0) FROM time_entries WHERE project_id = ? AND end_time IS NOT NULL", p.ID).Scan(&totalMinutes)
		p.TotalHours = float64(totalMinutes) / 60

		if p.Rate > 0 {
			p.TotalAmount = p.TotalHours * p.Rate
		}

		p.Tags = getProjectTags(p.ID)
		projects = append(projects, p)
	}

	var clients []models.Client
	clientRows, _ := db.DB.Query("SELECT id, company_name FROM clients ORDER BY company_name")
	for clientRows.Next() {
		var c models.Client
		clientRows.Scan(&c.ID, &c.CompanyName)
		clients = append(clients, c)
	}
	clientRows.Close()

	var tags []models.Tag
	tagRows, _ := db.DB.Query("SELECT id, name, color FROM project_tags ORDER BY name")
	for tagRows.Next() {
		var t models.Tag
		tagRows.Scan(&t.ID, &t.Name, &t.Color)
		tags = append(tags, t)
	}
	tagRows.Close()

	renderTemplate(w, "projects.html", TemplateData{
		Title:  "项目管理",
		User:   user,
		Data:   map[string]interface{}{"projects": projects, "clients": clients, "tags": tags, "search": search, "statusFilter": statusFilter, "typeFilter": typeFilter, "tagFilter": tagFilter},
		Active: "projects",
	})
}

func getProjectTags(projectID int64) []models.Tag {
	rows, err := db.DB.Query(`SELECT t.id, t.name, t.color FROM project_tags t 
		INNER JOIN project_tag_maps tm ON t.id = tm.tag_id WHERE tm.project_id = ?`, projectID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var tags []models.Tag
	for rows.Next() {
		var t models.Tag
		rows.Scan(&t.ID, &t.Name, &t.Color)
		tags = append(tags, t)
	}
	return tags
}

func NewProjectForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)

	var clients []models.Client
	clientRows, _ := db.DB.Query("SELECT id, company_name FROM clients ORDER BY company_name")
	for clientRows.Next() {
		var c models.Client
		clientRows.Scan(&c.ID, &c.CompanyName)
		clients = append(clients, c)
	}
	clientRows.Close()

	var tags []models.Tag
	tagRows, _ := db.DB.Query("SELECT id, name, color FROM project_tags ORDER BY name")
	for tagRows.Next() {
		var t models.Tag
		tagRows.Scan(&t.ID, &t.Name, &t.Color)
		tags = append(tags, t)
	}
	tagRows.Close()

	renderTemplate(w, "project_form.html", TemplateData{
		Title:  "新建项目",
		User:   user,
		Data:   map[string]interface{}{"clients": clients, "tags": tags, "project": nil},
		Active: "projects",
	})
}

func CreateProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/projects/new", http.StatusSeeOther)
		return
	}

	clientID := parseInt64(r.FormValue("client_id"))
	name := r.FormValue("name")
	description := r.FormValue("description")
	projType := r.FormValue("type")
	status := r.FormValue("status")
	rate := parseFloat(r.FormValue("rate"))
	estimatedHours := parseFloat(r.FormValue("estimated_hours"))
	budget := parseFloat(r.FormValue("budget"))
	startDate := r.FormValue("start_date")
	dueDate := r.FormValue("due_date")
	tagIDs := r.Form["tag_ids"]

	result, err := db.DB.Exec(`INSERT INTO projects (client_id, name, description, type, status, rate, estimated_hours, budget, start_date, due_date) 
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, clientID, name, description, projType, status, rate, estimatedHours, budget, startDate, dueDate)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	projectID, _ := result.LastInsertId()

	for _, tagID := range tagIDs {
		db.DB.Exec("INSERT OR IGNORE INTO project_tag_maps (project_id, tag_id) VALUES (?, ?)", projectID, parseInt64(tagID))
	}

	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

func EditProjectForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	id := parseInt64(r.URL.Query().Get("id"))

	var p models.Project
	err := db.DB.QueryRow(`SELECT id, client_id, name, description, type, status, rate, estimated_hours, budget, start_date, due_date 
		FROM projects WHERE id = ?`, id).Scan(&p.ID, &p.ClientID, &p.Name, &p.Description, &p.Type, &p.Status,
		&p.Rate, &p.EstimatedHours, &p.Budget, &p.StartDate, &p.DueDate)
	if err != nil {
		http.Error(w, "项目不存在", http.StatusNotFound)
		return
	}

	var clients []models.Client
	clientRows, _ := db.DB.Query("SELECT id, company_name FROM clients ORDER BY company_name")
	for clientRows.Next() {
		var c models.Client
		clientRows.Scan(&c.ID, &c.CompanyName)
		clients = append(clients, c)
	}
	clientRows.Close()

	var tags []models.Tag
	tagRows, _ := db.DB.Query("SELECT id, name, color FROM project_tags ORDER BY name")
	for tagRows.Next() {
		var t models.Tag
		tagRows.Scan(&t.ID, &t.Name, &t.Color)
		tags = append(tags, t)
	}
	tagRows.Close()

	selectedTagIDs := getSelectedTagIDs("project_tag_maps", "project_id", id)

	renderTemplate(w, "project_form.html", TemplateData{
		Title:  "编辑项目",
		User:   user,
		Data:   map[string]interface{}{"project": p, "clients": clients, "tags": tags, "selectedTagIDs": selectedTagIDs},
		Active: "projects",
	})
}

func UpdateProject(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}

	id := parseInt64(r.FormValue("id"))
	clientID := parseInt64(r.FormValue("client_id"))
	name := r.FormValue("name")
	description := r.FormValue("description")
	projType := r.FormValue("type")
	status := r.FormValue("status")
	rate := parseFloat(r.FormValue("rate"))
	estimatedHours := parseFloat(r.FormValue("estimated_hours"))
	budget := parseFloat(r.FormValue("budget"))
	startDate := r.FormValue("start_date")
	dueDate := r.FormValue("due_date")
	tagIDs := r.Form["tag_ids"]

	_, err := db.DB.Exec(`UPDATE projects SET client_id = ?, name = ?, description = ?, type = ?, status = ?, rate = ?, estimated_hours = ?, budget = ?, start_date = ?, due_date = ? 
		WHERE id = ?`, clientID, name, description, projType, status, rate, estimatedHours, budget, startDate, dueDate, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	db.DB.Exec("DELETE FROM project_tag_maps WHERE project_id = ?", id)
	for _, tagID := range tagIDs {
		db.DB.Exec("INSERT OR IGNORE INTO project_tag_maps (project_id, tag_id) VALUES (?, ?)", id, parseInt64(tagID))
	}

	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

func DeleteProject(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	db.DB.Exec("DELETE FROM projects WHERE id = ?", id)
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

func ProjectDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	id := parseInt64(r.URL.Query().Get("id"))

	var p models.Project
	db.DB.QueryRow(`SELECT p.id, p.client_id, c.company_name, p.name, p.description, p.type, p.status, p.rate, p.estimated_hours, p.budget, p.start_date, p.due_date, p.created_at, p.updated_at 
		FROM projects p LEFT JOIN clients c ON p.client_id = c.id WHERE p.id = ?`, id).Scan(&p.ID, &p.ClientID, &p.ClientName, &p.Name, &p.Description, &p.Type, &p.Status,
		&p.Rate, &p.EstimatedHours, &p.Budget, &p.StartDate, &p.DueDate, &p.CreatedAt, &p.UpdatedAt)

	p.Tags = getProjectTags(id)

	var totalMinutes int
	db.DB.QueryRow("SELECT COALESCE(SUM(duration_minutes), 0) FROM time_entries WHERE project_id = ? AND end_time IS NOT NULL", id).Scan(&totalMinutes)
	p.TotalHours = float64(totalMinutes) / 60
	if p.Rate > 0 {
		p.TotalAmount = p.TotalHours * p.Rate
	}

	milestoneRows, _ := db.DB.Query(`SELECT id, project_id, title, description, due_date, deliverable, status, created_at 
		FROM milestones WHERE project_id = ? ORDER BY COALESCE(due_date, '9999-12-31')`, id)
	var milestones []models.Milestone
	for milestoneRows.Next() {
		var m models.Milestone
		milestoneRows.Scan(&m.ID, &m.ProjectID, &m.Title, &m.Description, &m.DueDate, &m.Deliverable, &m.Status, &m.CreatedAt)
		milestones = append(milestones, m)
	}
	milestoneRows.Close()

	fileRows, _ := db.DB.Query(`SELECT id, project_id, filename, original_name, size, file_type, category, uploaded_at 
		FROM project_files WHERE project_id = ? ORDER BY uploaded_at DESC`, id)
	var files []models.ProjectFile
	for fileRows.Next() {
		var f models.ProjectFile
		fileRows.Scan(&f.ID, &f.ProjectID, &f.Filename, &f.OriginalName, &f.Size, &f.FileType, &f.Category, &f.UploadedAt)
		files = append(files, f)
	}
	fileRows.Close()

	timeRows, _ := db.DB.Query(`SELECT id, description, start_time, end_time, duration_minutes, rate, created_at 
		FROM time_entries WHERE project_id = ? ORDER BY start_time DESC LIMIT 50`, id)
	var timeEntries []models.TimeEntry
	for timeRows.Next() {
		var t models.TimeEntry
		var endTime *time.Time
		timeRows.Scan(&t.ID, &t.Description, &t.StartTime, &endTime, &t.DurationMinutes, &t.Rate, &t.CreatedAt)
		t.EndTime = endTime
		timeEntries = append(timeEntries, t)
	}
	timeRows.Close()

	var activeTimer *models.TimeEntry
	timerRow := db.DB.QueryRow(`SELECT id, description, start_time, rate FROM time_entries WHERE project_id = ? AND end_time IS NULL LIMIT 1`, id)
	var t models.TimeEntry
	err := timerRow.Scan(&t.ID, &t.Description, &t.StartTime, &t.Rate)
	if err == nil {
		activeTimer = &t
	}

	renderTemplate(w, "project_detail.html", TemplateData{
		Title:  p.Name,
		User:   user,
		Data:   map[string]interface{}{"project": p, "milestones": milestones, "files": files, "timeEntries": timeEntries, "activeTimer": activeTimer},
		Active: "projects",
	})
}

func CreateMilestone(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}

	projectID := parseInt64(r.FormValue("project_id"))
	title := r.FormValue("title")
	description := r.FormValue("description")
	dueDate := r.FormValue("due_date")
	deliverable := r.FormValue("deliverable")

	db.DB.Exec(`INSERT INTO milestones (project_id, title, description, due_date, deliverable) VALUES (?, ?, ?, ?, ?)`,
		projectID, title, description, dueDate, deliverable)

	http.Redirect(w, r, "/projects/detail?id="+r.FormValue("project_id"), http.StatusSeeOther)
}

func UpdateMilestoneStatus(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	status := r.URL.Query().Get("status")
	projectID := r.URL.Query().Get("project_id")

	db.DB.Exec("UPDATE milestones SET status = ? WHERE id = ?", status, id)
	http.Redirect(w, r, "/projects/detail?id="+projectID, http.StatusSeeOther)
}

func DeleteMilestone(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	projectID := r.URL.Query().Get("project_id")
	db.DB.Exec("DELETE FROM milestones WHERE id = ?", id)
	http.Redirect(w, r, "/projects/detail?id="+projectID, http.StatusSeeOther)
}

func UploadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/projects", http.StatusSeeOther)
		return
	}

	projectID := parseInt64(r.FormValue("project_id"))
	category := r.FormValue("category")
	if category == "" {
		category = "其他"
	}

	r.ParseMultipartForm(32 << 20)

	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Error uploading file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	uploadDir := filepath.Join(config.UploadDir, "projects", strconv.FormatInt(projectID, 10))
	os.MkdirAll(uploadDir, 0755)

	ext := filepath.Ext(handler.Filename)
	filename := strconv.FormatInt(time.Now().UnixNano(), 10) + ext
	dst := filepath.Join(uploadDir, filename)

	dstFile, err := os.Create(dst)
	if err != nil {
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}
	defer dstFile.Close()

	io.Copy(dstFile, file)

	db.DB.Exec(`INSERT INTO project_files (project_id, filename, original_name, size, file_type, category) 
		VALUES (?, ?, ?, ?, ?, ?)`, projectID, filename, handler.Filename, handler.Size, ext[1:], category)

	http.Redirect(w, r, "/projects/detail?id="+r.FormValue("project_id"), http.StatusSeeOther)
}

func DownloadFile(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))

	var f models.ProjectFile
	db.DB.QueryRow("SELECT project_id, filename, original_name, file_type FROM project_files WHERE id = ?", id).
		Scan(&f.ProjectID, &f.Filename, &f.OriginalName, &f.FileType)

	filePath := filepath.Join(config.UploadDir, "projects", strconv.FormatInt(f.ProjectID, 10), f.Filename)

	w.Header().Set("Content-Disposition", "attachment; filename="+f.OriginalName)
	w.Header().Set("Content-Type", "application/octet-stream")

	http.ServeFile(w, r, filePath)
}

func DeleteFile(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	projectID := r.URL.Query().Get("project_id")

	var filename string
	var projID int64
	db.DB.QueryRow("SELECT project_id, filename FROM project_files WHERE id = ?", id).Scan(&projID, &filename)

	filePath := filepath.Join(config.UploadDir, "projects", strconv.FormatInt(projID, 10), filename)
	os.Remove(filePath)

	db.DB.Exec("DELETE FROM project_files WHERE id = ?", id)
	http.Redirect(w, r, "/projects/detail?id="+projectID, http.StatusSeeOther)
}

func ManageProjectTags(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)

	if r.Method == "POST" {
		name := r.FormValue("name")
		color := r.FormValue("color")
		if name != "" {
			db.DB.Exec("INSERT OR IGNORE INTO project_tags (name, color) VALUES (?, ?)", name, color)
		}
	}

	var tags []models.Tag
	rows, _ := db.DB.Query("SELECT id, name, color FROM project_tags ORDER BY name")
	for rows.Next() {
		var t models.Tag
		rows.Scan(&t.ID, &t.Name, &t.Color)
		tags = append(tags, t)
	}
	rows.Close()

	renderTemplate(w, "project_tags.html", TemplateData{
		Title:  "项目标签管理",
		User:   user,
		Data:   tags,
		Active: "projects",
	})
}

func DeleteProjectTag(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	db.DB.Exec("DELETE FROM project_tags WHERE id = ?", id)
	http.Redirect(w, r, "/projects/tags", http.StatusSeeOther)
}

func UpdateProjectStatus(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	status := r.URL.Query().Get("status")

	db.DB.Exec("UPDATE projects SET status = ? WHERE id = ?", status, id)
	http.Redirect(w, r, "/projects", http.StatusSeeOther)
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
