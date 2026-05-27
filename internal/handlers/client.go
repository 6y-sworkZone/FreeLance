package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/freelance/workbench/internal/db"
	"github.com/freelance/workbench/internal/middleware"
	"github.com/freelance/workbench/internal/models"
)

func parseFloat(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

func parseInt(s string) int {
	i, _ := strconv.Atoi(s)
	return i
}

func parseInt64(s string) int64 {
	i, _ := strconv.ParseInt(s, 10, 64)
	return i
}

func ListClients(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	search := r.URL.Query().Get("search")
	tagFilter := r.URL.Query().Get("tag")
	sourceFilter := r.URL.Query().Get("source")

	query := `SELECT c.id, c.company_name, c.contact_person, c.email, c.phone, c.industry, c.source, c.notes, c.value_score, c.created_at, c.updated_at
		FROM clients c WHERE 1=1`
	var args []interface{}

	if search != "" {
		query += " AND (c.company_name LIKE ? OR c.contact_person LIKE ? OR c.email LIKE ?)"
		searchLike := "%" + search + "%"
		args = append(args, searchLike, searchLike, searchLike)
	}
	if sourceFilter != "" {
		query += " AND c.source = ?"
		args = append(args, sourceFilter)
	}
	if tagFilter != "" {
		query += " AND c.id IN (SELECT client_id FROM client_tag_maps WHERE tag_id = ?)"
		args = append(args, parseInt64(tagFilter))
	}

	query += " ORDER BY c.created_at DESC"

	rows, err := db.DB.Query(query, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var clients []models.Client
	for rows.Next() {
		var c models.Client
		rows.Scan(&c.ID, &c.CompanyName, &c.ContactPerson, &c.Email, &c.Phone,
			&c.Industry, &c.Source, &c.Notes, &c.ValueScore, &c.CreatedAt, &c.UpdatedAt)

		var projectCount int
		var totalRevenue float64
		db.DB.QueryRow("SELECT COUNT(*) FROM projects WHERE client_id = ?", c.ID).Scan(&projectCount)
		db.DB.QueryRow(`SELECT COALESCE(SUM(total), 0) FROM invoices WHERE client_id = ? AND status = '已支付'`, c.ID).Scan(&totalRevenue)
		c.ProjectCount = projectCount
		c.TotalRevenue = totalRevenue

		c.ValueScore = calculateClientValueScore(c.ID)

		c.Tags = getClientTags(c.ID)
		clients = append(clients, c)
	}

	var tags []models.Tag
	rows, _ = db.DB.Query("SELECT id, name, color FROM client_tags ORDER BY name")
	for rows.Next() {
		var t models.Tag
		rows.Scan(&t.ID, &t.Name, &t.Color)
		tags = append(tags, t)
	}
	rows.Close()

	renderTemplate(w, "clients.html", TemplateData{
		Title:  "客户管理",
		User:   user,
		Data:   map[string]interface{}{"clients": clients, "tags": tags, "search": search, "tagFilter": tagFilter, "sourceFilter": sourceFilter},
		Active: "clients",
	})
}

func calculateClientValueScore(clientID int64) float64 {
	var projectCount int
	var totalRevenue float64
	var avgPaymentDays float64

	db.DB.QueryRow("SELECT COUNT(*) FROM projects WHERE client_id = ?", clientID).Scan(&projectCount)
	db.DB.QueryRow(`SELECT COALESCE(SUM(total), 0) FROM invoices WHERE client_id = ? AND status = '已支付'`, clientID).Scan(&totalRevenue)

	revenueScore := totalRevenue / 100000 * 40
	if revenueScore > 40 {
		revenueScore = 40
	}
	projectScore := float64(projectCount) * 10
	if projectScore > 30 {
		projectScore = 30
	}

	_ = avgPaymentDays

	return revenueScore + projectScore + 30
}

func getClientTags(clientID int64) []models.Tag {
	rows, err := db.DB.Query(`SELECT t.id, t.name, t.color FROM client_tags t 
		INNER JOIN client_tag_maps tm ON t.id = tm.tag_id WHERE tm.client_id = ?`, clientID)
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

func NewClientForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	var tags []models.Tag
	rows, _ := db.DB.Query("SELECT id, name, color FROM client_tags ORDER BY name")
	for rows.Next() {
		var t models.Tag
		rows.Scan(&t.ID, &t.Name, &t.Color)
		tags = append(tags, t)
	}
	rows.Close()

	renderTemplate(w, "client_form.html", TemplateData{
		Title:  "新建客户",
		User:   user,
		Data:   map[string]interface{}{"tags": tags, "client": nil},
		Active: "clients",
	})
}

func CreateClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/clients/new", http.StatusSeeOther)
		return
	}

	companyName := r.FormValue("company_name")
	contactPerson := r.FormValue("contact_person")
	email := r.FormValue("email")
	phone := r.FormValue("phone")
	industry := r.FormValue("industry")
	source := r.FormValue("source")
	notes := r.FormValue("notes")
	tagIDs := r.Form["tag_ids"]

	result, err := db.DB.Exec(`INSERT INTO clients (company_name, contact_person, email, phone, industry, source, notes) 
		VALUES (?, ?, ?, ?, ?, ?, ?)`, companyName, contactPerson, email, phone, industry, source, notes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	clientID, _ := result.LastInsertId()

	for _, tagID := range tagIDs {
		db.DB.Exec("INSERT OR IGNORE INTO client_tag_maps (client_id, tag_id) VALUES (?, ?)", clientID, parseInt64(tagID))
	}

	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

func EditClientForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	id := parseInt64(r.URL.Query().Get("id"))

	var c models.Client
	err := db.DB.QueryRow(`SELECT id, company_name, contact_person, email, phone, industry, source, notes, value_score, created_at, updated_at 
		FROM clients WHERE id = ?`, id).Scan(&c.ID, &c.CompanyName, &c.ContactPerson, &c.Email, &c.Phone,
		&c.Industry, &c.Source, &c.Notes, &c.ValueScore, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		http.Error(w, "客户不存在", http.StatusNotFound)
		return
	}

	var tags []models.Tag
	rows, _ := db.DB.Query("SELECT id, name, color FROM client_tags ORDER BY name")
	for rows.Next() {
		var t models.Tag
		rows.Scan(&t.ID, &t.Name, &t.Color)
		tags = append(tags, t)
	}
	rows.Close()

	selectedTagIDs := getSelectedTagIDs("client_tag_maps", "client_id", id)

	renderTemplate(w, "client_form.html", TemplateData{
		Title:  "编辑客户",
		User:   user,
		Data:   map[string]interface{}{"client": c, "tags": tags, "selectedTagIDs": selectedTagIDs},
		Active: "clients",
	})
}

func getSelectedTagIDs(table, column string, id int64) []int64 {
	rows, err := db.DB.Query("SELECT tag_id FROM "+table+" WHERE "+column+" = ?", id)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var tid int64
		rows.Scan(&tid)
		ids = append(ids, tid)
	}
	return ids
}

func UpdateClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/clients", http.StatusSeeOther)
		return
	}

	id := parseInt64(r.FormValue("id"))
	companyName := r.FormValue("company_name")
	contactPerson := r.FormValue("contact_person")
	email := r.FormValue("email")
	phone := r.FormValue("phone")
	industry := r.FormValue("industry")
	source := r.FormValue("source")
	notes := r.FormValue("notes")
	tagIDs := r.Form["tag_ids"]

	_, err := db.DB.Exec(`UPDATE clients SET company_name = ?, contact_person = ?, email = ?, phone = ?, industry = ?, source = ?, notes = ? 
		WHERE id = ?`, companyName, contactPerson, email, phone, industry, source, notes, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	db.DB.Exec("DELETE FROM client_tag_maps WHERE client_id = ?", id)
	for _, tagID := range tagIDs {
		db.DB.Exec("INSERT OR IGNORE INTO client_tag_maps (client_id, tag_id) VALUES (?, ?)", id, parseInt64(tagID))
	}

	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

func DeleteClient(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	db.DB.Exec("DELETE FROM clients WHERE id = ?", id)
	http.Redirect(w, r, "/clients", http.StatusSeeOther)
}

func ClientDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	id := parseInt64(r.URL.Query().Get("id"))

	var c models.Client
	db.DB.QueryRow(`SELECT id, company_name, contact_person, email, phone, industry, source, notes, value_score, created_at, updated_at 
		FROM clients WHERE id = ?`, id).Scan(&c.ID, &c.CompanyName, &c.ContactPerson, &c.Email, &c.Phone,
		&c.Industry, &c.Source, &c.Notes, &c.ValueScore, &c.CreatedAt, &c.UpdatedAt)

	c.Tags = getClientTags(id)
	c.ValueScore = calculateClientValueScore(id)

	var projectCount int
	var totalRevenue float64
	db.DB.QueryRow("SELECT COUNT(*) FROM projects WHERE client_id = ?", id).Scan(&projectCount)
	db.DB.QueryRow(`SELECT COALESCE(SUM(total), 0) FROM invoices WHERE client_id = ? AND status = '已支付'`, id).Scan(&totalRevenue)
	c.ProjectCount = projectCount
	c.TotalRevenue = totalRevenue

	commRows, _ := db.DB.Query(`SELECT id, client_id, type, content, occurred_at, created_at 
		FROM communications WHERE client_id = ? ORDER BY occurred_at DESC`, id)
	var communications []models.Communication
	for commRows.Next() {
		var comm models.Communication
		commRows.Scan(&comm.ID, &comm.ClientID, &comm.Type, &comm.Content, &comm.OccurredAt, &comm.CreatedAt)
		communications = append(communications, comm)
	}
	commRows.Close()

	projRows, _ := db.DB.Query(`SELECT id, name, status, type, due_date FROM projects WHERE client_id = ? ORDER BY created_at DESC`, id)
	var projects []struct {
		ID      int64
		Name    string
		Status  string
		Type    string
		DueDate string
	}
	for projRows.Next() {
		var p struct {
			ID      int64
			Name    string
			Status  string
			Type    string
			DueDate string
		}
		projRows.Scan(&p.ID, &p.Name, &p.Status, &p.Type, &p.DueDate)
		projects = append(projects, p)
	}
	projRows.Close()

	renderTemplate(w, "client_detail.html", TemplateData{
		Title:  c.CompanyName,
		User:   user,
		Data:   map[string]interface{}{"client": c, "communications": communications, "projects": projects},
		Active: "clients",
	})
}

func AddCommunication(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Redirect(w, r, "/clients", http.StatusSeeOther)
		return
	}

	clientID := parseInt64(r.FormValue("client_id"))
	commType := r.FormValue("type")
	content := r.FormValue("content")
	occurredAt := r.FormValue("occurred_at")

	db.DB.Exec(`INSERT INTO communications (client_id, type, content, occurred_at) VALUES (?, ?, ?, ?)`,
		clientID, commType, content, occurredAt)

	http.Redirect(w, r, "/clients/detail?id="+r.FormValue("client_id"), http.StatusSeeOther)
}

func DeleteCommunication(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	clientID := r.URL.Query().Get("client_id")
	db.DB.Exec("DELETE FROM communications WHERE id = ?", id)
	http.Redirect(w, r, "/clients/detail?id="+clientID, http.StatusSeeOther)
}

func ManageClientTags(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)

	if r.Method == "POST" {
		name := r.FormValue("name")
		color := r.FormValue("color")
		if name != "" {
			db.DB.Exec("INSERT OR IGNORE INTO client_tags (name, color) VALUES (?, ?)", name, color)
		}
	}

	var tags []models.Tag
	rows, _ := db.DB.Query("SELECT id, name, color FROM client_tags ORDER BY name")
	for rows.Next() {
		var t models.Tag
		rows.Scan(&t.ID, &t.Name, &t.Color)
		tags = append(tags, t)
	}
	rows.Close()

	renderTemplate(w, "client_tags.html", TemplateData{
		Title:  "客户标签管理",
		User:   user,
		Data:   tags,
		Active: "clients",
	})
}

func DeleteClientTag(w http.ResponseWriter, r *http.Request) {
	id := parseInt64(r.URL.Query().Get("id"))
	db.DB.Exec("DELETE FROM client_tags WHERE id = ?", id)
	http.Redirect(w, r, "/clients/tags", http.StatusSeeOther)
}

func hasString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func joinStrings(strs []string, sep string) string {
	return strings.Join(strs, sep)
}
